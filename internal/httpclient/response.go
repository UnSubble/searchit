package httpclient

import "net/http"

// ContentLength returns resp.ContentLength, which is -1 when the header is absent.
func ContentLength(resp *http.Response) int64 {
	return resp.ContentLength
}
