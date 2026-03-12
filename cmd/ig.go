package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cwygoda/ansel/internal/config"
	imglib "github.com/cwygoda/ansel/internal/image"
	"github.com/cwygoda/ansel/internal/instagram"
	"github.com/cwygoda/ansel/internal/tunnel"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const (
	igMaxWidth       = 1080
	containerTimeout = 2 * time.Minute
)

var igCmd = &cobra.Command{
	Use:   "ig [flags] <image>...",
	Short: "Publish images to Instagram",
	Long: `Publish images to Instagram using the Graph API.

Requires configuration in ~/.ansel/config.toml:

  [instagram]
  access_token = "your-access-token"
  user_id = "your-instagram-user-id"
  ngrok_token = "your-ngrok-auth-token"

Images larger than 1080px wide are automatically resized using MKS2021 filter
for better quality than Instagram's built-in downscaling.

Examples:
  # Publish a single image (prompts for caption)
  ansel ig photo.jpg

  # Publish multiple images as a carousel
  ansel ig --carousel photo1.jpg photo2.jpg photo3.jpg

  # Publish with pre-set caption
  ansel ig --caption "Hello world!" photo.jpg

  # Dry run (prepare but don't publish)
  ansel ig --dry-run photo.jpg`,
	Args: cobra.MinimumNArgs(1),
	RunE: runIG,
}

var (
	igCarousel bool
	igCaption  string
	igDryRun   bool
)

func init() {
	rootCmd.AddCommand(igCmd)

	igCmd.Flags().BoolVar(&igCarousel, "carousel", false, "Publish multiple images as a carousel (up to 10)")
	igCmd.Flags().StringVar(&igCaption, "caption", "", "Caption for the post (prompts if not set)")
	igCmd.Flags().BoolVar(&igDryRun, "dry-run", false, "Prepare images but don't publish")
}

func runIG(cmd *cobra.Command, args []string) error {
	// Validate carousel size
	if igCarousel && len(args) > 10 {
		return fmt.Errorf("carousel supports at most 10 images, got %d", len(args))
	}
	if igCarousel && len(args) < 2 {
		return fmt.Errorf("carousel requires at least 2 images")
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	// Initialize vips
	imglib.InitVips()
	defer imglib.ShutdownVips()

	// Prepare images (resize if needed)
	fmt.Fprintln(os.Stderr, "Preparing images...")
	preparedFiles, err := prepareImages(args)
	if err != nil {
		cleanupTempFiles(preparedFiles)
		return err
	}
	defer cleanupTempFiles(preparedFiles)

	// Prompt for caption if not provided
	caption := igCaption
	if caption == "" {
		var err error
		caption, err = promptCaption()
		if err != nil {
			return err
		}
	}

	if igDryRun {
		fmt.Fprintln(os.Stderr, "\nDry run mode - not publishing")
		fmt.Fprintf(os.Stderr, "Would publish %d image(s) with caption: %s\n", len(preparedFiles), caption)
		return nil
	}

	// Start ngrok tunnel
	fmt.Fprintln(os.Stderr, "Starting tunnel...")
	ctx := context.Background()
	server, err := tunnel.New(ctx, cfg.NgrokAuthToken())
	if err != nil {
		return fmt.Errorf("failed to start tunnel: %w", err)
	}
	defer server.Close()

	fmt.Fprintf(os.Stderr, "Tunnel URL: %s\n", server.URL())

	// Add files to server and get public URLs
	publicURLs := make([]string, len(preparedFiles))
	for i, file := range preparedFiles {
		url, err := server.AddFile(file)
		if err != nil {
			return fmt.Errorf("failed to add file to server: %w", err)
		}
		publicURLs[i] = url
		fmt.Fprintf(os.Stderr, "  %s -> %s\n", args[i], url)
	}

	// Create Instagram client
	client := instagram.NewClient(cfg.Instagram.AccessToken, cfg.Instagram.UserID)

	// Publish based on mode
	if igCarousel && len(args) > 1 {
		return publishCarousel(client, publicURLs, caption)
	}
	return publishSingle(client, publicURLs, caption)
}

// prepareImages loads and resizes images if needed, returns temp file paths.
func prepareImages(inputPaths []string) ([]string, error) {
	prepared := make([]string, 0, len(inputPaths))

	for _, inputPath := range inputPaths {
		img, err := imglib.LoadVips(inputPath)
		if err != nil {
			return prepared, fmt.Errorf("failed to load %s: %w", inputPath, err)
		}

		// Resize if width > 1080
		if img.Width() > igMaxWidth {
			scale := float64(igMaxWidth) / float64(img.Width())
			newHeight := int(float64(img.Height()) * scale)
			if err := img.ResizeToFit(igMaxWidth, newHeight, imglib.MagicKernelSharp2021); err != nil {
				img.Close()
				return prepared, fmt.Errorf("failed to resize %s: %w", inputPath, err)
			}
			fmt.Fprintf(os.Stderr, "  %s: resized to %dx%d\n", inputPath, img.Width(), img.Height())
		} else {
			fmt.Fprintf(os.Stderr, "  %s: %dx%d (no resize needed)\n", inputPath, img.Width(), img.Height())
		}

		// Save to temp file
		tmpFile, err := os.CreateTemp("", "ansel-ig-*.jpg")
		if err != nil {
			img.Close()
			return prepared, fmt.Errorf("failed to create temp file: %w", err)
		}
		tmpPath := tmpFile.Name()
		tmpFile.Close()

		if err := img.SaveJPEG(tmpPath, 92); err != nil {
			img.Close()
			os.Remove(tmpPath)
			return prepared, fmt.Errorf("failed to save %s: %w", inputPath, err)
		}
		img.Close()

		prepared = append(prepared, tmpPath)
	}

	return prepared, nil
}

// cleanupTempFiles removes temporary files.
func cleanupTempFiles(files []string) {
	for _, f := range files {
		os.Remove(f)
	}
}

// promptCaption prompts the user for a caption.
func promptCaption() (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("caption required in non-interactive mode; use --caption flag")
	}

	fmt.Fprint(os.Stderr, "Enter caption: ")
	reader := bufio.NewReader(os.Stdin)
	caption, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read caption: %w", err)
	}

	return strings.TrimSpace(caption), nil
}

// publishSingle publishes images as individual posts.
func publishSingle(client *instagram.Client, urls []string, caption string) error {
	for i, url := range urls {
		fmt.Fprintf(os.Stderr, "\nPublishing image %d/%d...\n", i+1, len(urls))

		// Create container
		containerID, err := client.CreateImageContainer(url, caption)
		if err != nil {
			return fmt.Errorf("failed to create container: %w", err)
		}
		fmt.Fprintf(os.Stderr, "  Container ID: %s\n", containerID)

		// Wait for container to be ready
		fmt.Fprintln(os.Stderr, "  Waiting for processing...")
		if err := client.WaitForContainer(containerID, containerTimeout); err != nil {
			return fmt.Errorf("container processing failed: %w", err)
		}

		// Publish
		mediaID, err := client.Publish(containerID)
		if err != nil {
			return fmt.Errorf("failed to publish: %w", err)
		}
		fmt.Fprintf(os.Stderr, "  Published! Media ID: %s\n", mediaID)
	}

	fmt.Fprintln(os.Stderr, "\nDone!")
	return nil
}

// publishCarousel publishes multiple images as a carousel.
func publishCarousel(client *instagram.Client, urls []string, caption string) error {
	fmt.Fprintln(os.Stderr, "\nCreating carousel items...")

	// Create child containers for each image
	childIDs := make([]string, len(urls))
	for i, url := range urls {
		containerID, err := client.CreateCarouselItem(url)
		if err != nil {
			return fmt.Errorf("failed to create carousel item %d: %w", i+1, err)
		}
		childIDs[i] = containerID
		fmt.Fprintf(os.Stderr, "  Item %d: %s\n", i+1, containerID)

		// Wait for each item to be ready
		if err := client.WaitForContainer(containerID, containerTimeout); err != nil {
			return fmt.Errorf("carousel item %d processing failed: %w", i+1, err)
		}
	}

	// Create carousel container
	fmt.Fprintln(os.Stderr, "Creating carousel...")
	carouselID, err := client.CreateCarouselContainer(childIDs, caption)
	if err != nil {
		return fmt.Errorf("failed to create carousel: %w", err)
	}
	fmt.Fprintf(os.Stderr, "  Carousel ID: %s\n", carouselID)

	// Wait for carousel container
	fmt.Fprintln(os.Stderr, "Waiting for processing...")
	if err := client.WaitForContainer(carouselID, containerTimeout); err != nil {
		return fmt.Errorf("carousel processing failed: %w", err)
	}

	// Publish
	mediaID, err := client.Publish(carouselID)
	if err != nil {
		return fmt.Errorf("failed to publish carousel: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Published! Media ID: %s\n", mediaID)

	fmt.Fprintln(os.Stderr, "\nDone!")
	return nil
}
