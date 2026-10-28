package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestStaticServesVendorModulesWithJavaScriptMIME(t *testing.T) {
	ts, _ := newTestServer(t)
	client := testClient(t, ts)

	for _, path := range []string{"/vendor/pdfjs/pdf.min.mjs", "/vendor/pdfjs/pdf.worker.min.mjs"} {
		res, err := client.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", path, res.StatusCode)
		}
		ct := res.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "text/javascript") {
			t.Fatalf("%s Content-Type = %q, want text/javascript", path, ct)
		}
	}
}
