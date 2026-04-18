package server

import (
	"net/http"
	"testing"

	v1 "caichip/api/bom/v1"
)

func TestHsResolveByModelHTTPStatus(t *testing.T) {
	t.Parallel()
	if got := HsResolveByModelHTTPStatus(&v1.HsResolveByModelReply{Accepted: true}); got != http.StatusAccepted {
		t.Fatalf("accepted async expected 202, got %d", got)
	}
	if got := HsResolveByModelHTTPStatus(&v1.HsResolveByModelReply{Accepted: false}); got != http.StatusOK {
		t.Fatalf("sync reply expected 200, got %d", got)
	}
	if got := HsResolveByModelHTTPStatus(nil); got != http.StatusOK {
		t.Fatalf("nil reply expected 200, got %d", got)
	}
}
