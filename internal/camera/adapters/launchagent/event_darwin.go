package launchagent

/*
#cgo darwin CFLAGS: -x objective-c -fblocks
#cgo darwin LDFLAGS: -framework Foundation
#include <dispatch/dispatch.h>
#include <xpc/xpc.h>

static int ansel_consume_iokit_event(int timeout_seconds) {
	dispatch_semaphore_t sem = dispatch_semaphore_create(0);
	dispatch_queue_t q = dispatch_queue_create("com.cwygoda.ansel.camera-import.xpc", NULL);
	xpc_set_event_stream_handler("com.apple.iokit.matching", q, ^(xpc_object_t event) {
		dispatch_semaphore_signal(sem);
	});
	long result = dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, (int64_t)timeout_seconds * NSEC_PER_SEC));
	return result == 0 ? 1 : 0;
}
*/
import "C"

// ConsumeIOKitEvent checks in with launchd's IOKit XPC event stream and consumes
// the pending USB attach event. A timeout is normal when run manually.
func ConsumeIOKitEvent(timeoutSeconds int) bool {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 5
	}
	return C.ansel_consume_iokit_event(C.int(timeoutSeconds)) == 1
}
