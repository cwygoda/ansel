package instagram

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	baseURL        = "https://graph.facebook.com/v21.0"
	defaultTimeout = 30 * time.Second
)

// Client is an Instagram Graph API client.
type Client struct {
	accessToken string
	userID      string
	httpClient  *http.Client
}

// NewClient creates a new Instagram API client.
func NewClient(accessToken, userID string) *Client {
	return &Client{
		accessToken: accessToken,
		userID:      userID,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

// APIError represents an error from the Instagram API.
type APIError struct {
	Message   string `json:"message"`
	Type      string `json:"type"`
	Code      int    `json:"code"`
	FBTraceID string `json:"fbtrace_id"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("Instagram API error %d: %s", e.Code, e.Message)
}

// apiResponse wraps API responses for error checking.
type apiResponse struct {
	ID    string `json:"id"`
	Error *struct {
		Message   string `json:"message"`
		Type      string `json:"type"`
		Code      int    `json:"code"`
		FBTraceID string `json:"fbtrace_id"`
	} `json:"error"`
}

// ContainerStatus represents the status of a media container.
type ContainerStatus struct {
	ID           string `json:"id"`
	StatusCode   string `json:"status_code"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// request makes an HTTP request to the API.
func (c *Client) request(method, endpoint string, params url.Values) ([]byte, error) {
	if params == nil {
		params = url.Values{}
	}
	params.Set("access_token", c.accessToken)

	var req *http.Request
	var err error

	fullURL := baseURL + endpoint

	if method == http.MethodGet {
		fullURL += "?" + params.Encode()
		req, err = http.NewRequest(method, fullURL, nil)
	} else {
		req, err = http.NewRequest(method, fullURL, strings.NewReader(params.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for API error
	var apiResp apiResponse
	if err := json.Unmarshal(body, &apiResp); err == nil && apiResp.Error != nil {
		return nil, &APIError{
			Message:   apiResp.Error.Message,
			Type:      apiResp.Error.Type,
			Code:      apiResp.Error.Code,
			FBTraceID: apiResp.Error.FBTraceID,
		}
	}

	return body, nil
}

// CreateImageContainer creates a container for a single image post.
func (c *Client) CreateImageContainer(imageURL, caption string) (string, error) {
	params := url.Values{}
	params.Set("image_url", imageURL)
	if caption != "" {
		params.Set("caption", caption)
	}

	body, err := c.request(http.MethodPost, "/"+c.userID+"/media", params)
	if err != nil {
		return "", fmt.Errorf("failed to create image container: %w", err)
	}

	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return resp.ID, nil
}

// CreateCarouselItem creates a child container for a carousel post.
func (c *Client) CreateCarouselItem(imageURL string) (string, error) {
	params := url.Values{}
	params.Set("image_url", imageURL)
	params.Set("is_carousel_item", "true")

	body, err := c.request(http.MethodPost, "/"+c.userID+"/media", params)
	if err != nil {
		return "", fmt.Errorf("failed to create carousel item: %w", err)
	}

	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return resp.ID, nil
}

// CreateCarouselContainer creates a parent container for a carousel post.
func (c *Client) CreateCarouselContainer(childIDs []string, caption string) (string, error) {
	params := url.Values{}
	params.Set("media_type", "CAROUSEL")
	params.Set("children", strings.Join(childIDs, ","))
	if caption != "" {
		params.Set("caption", caption)
	}

	body, err := c.request(http.MethodPost, "/"+c.userID+"/media", params)
	if err != nil {
		return "", fmt.Errorf("failed to create carousel container: %w", err)
	}

	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return resp.ID, nil
}

// GetContainerStatus checks the status of a media container.
func (c *Client) GetContainerStatus(containerID string) (*ContainerStatus, error) {
	params := url.Values{}
	params.Set("fields", "status_code")

	body, err := c.request(http.MethodGet, "/"+containerID, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get container status: %w", err)
	}

	var status ContainerStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, fmt.Errorf("failed to parse status: %w", err)
	}

	return &status, nil
}

// WaitForContainer polls until the container is ready or times out.
func (c *Client) WaitForContainer(containerID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	pollInterval := 2 * time.Second

	for time.Now().Before(deadline) {
		status, err := c.GetContainerStatus(containerID)
		if err != nil {
			return err
		}

		switch status.StatusCode {
		case "FINISHED":
			return nil
		case "ERROR":
			if status.ErrorMessage != "" {
				return fmt.Errorf("container error: %s", status.ErrorMessage)
			}
			return fmt.Errorf("container processing failed")
		case "IN_PROGRESS", "":
			time.Sleep(pollInterval)
			continue
		default:
			time.Sleep(pollInterval)
			continue
		}
	}

	return fmt.Errorf("timeout waiting for container %s", containerID)
}

// Publish publishes a media container.
func (c *Client) Publish(containerID string) (string, error) {
	params := url.Values{}
	params.Set("creation_id", containerID)

	body, err := c.request(http.MethodPost, "/"+c.userID+"/media_publish", params)
	if err != nil {
		return "", fmt.Errorf("failed to publish: %w", err)
	}

	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return resp.ID, nil
}
