package apis

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	defaultProbeBytes = 1024 * 1024     // 1MB
	maxProbeBytes     = 5 * 1024 * 1024 // 5MB
)

// BandwidthProbe serves a deterministic byte stream of N bytes so clients can
// measure their downlink throughput. Default 1MB, max 5MB, configurable via
// query parameter ?bytes=. Supports GET and HEAD.
func BandwidthProbe(c *gin.Context) {
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		c.Header("Allow", "GET, HEAD")
		c.AbortWithStatus(http.StatusMethodNotAllowed)
		return
	}
	size := parsePositiveIntParam(c.Query("bytes"), defaultProbeBytes, maxProbeBytes)

	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Length", fmt.Sprintf("%d", size))
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	c.Status(http.StatusOK)

	if c.Request.Method == http.MethodHead {
		return
	}
	_, _ = io.CopyN(c.Writer, zeroReader{}, int64(size))
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(i)
	}
	return len(p), nil
}

func parsePositiveIntParam(raw string, fallback int, max int) int {
	value := fallback
	if parsed, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && parsed > 0 {
		value = parsed
	}
	if max > 0 && value > max {
		return max
	}
	return value
}
