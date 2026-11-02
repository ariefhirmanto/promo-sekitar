package sheet

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"places-fetcher/config"
)

// fakeAppsScript answers like the deployed web app: a POST is redirected (302) to a GET that
// returns the result, and every answer is the {ok, ...} JSON envelope.
func fakeAppsScript(t *testing.T, handle func(mode string, q url.Values, body []byte) interface{}) *httptest.Server {
	t.Helper()
	var lastBody []byte
	mux := http.NewServeMux()
	mux.HandleFunc("/exec", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != "tok" {
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "unauthorized"})
			return
		}
		if r.Method == http.MethodPost {
			lastBody, _ = io.ReadAll(r.Body)
			http.Redirect(w, r, "/echo?"+r.URL.RawQuery, http.StatusFound)
			return
		}
		json.NewEncoder(w).Encode(handle(r.URL.Query().Get("mode"), r.URL.Query(), nil))
	})
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("redirect followed as %s, want GET", r.Method)
		}
		json.NewEncoder(w).Encode(handle(r.URL.Query().Get("mode"), r.URL.Query(), lastBody))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newTestClient(srv *httptest.Server, token string) *Client {
	return NewClient(config.SheetConfig{URL: srv.URL + "/exec", Token: token, TimeoutSeconds: 5})
}

func TestGetSendsModeAndParams(t *testing.T) {
	srv := fakeAppsScript(t, func(mode string, q url.Values, _ []byte) interface{} {
		return map[string]interface{}{"ok": true, "mode": mode, "status": q.Get("status")}
	})
	var got struct{ Mode, Status string }
	if err := newTestClient(srv, "tok").Get(context.Background(), "raw_items", url.Values{"status": {"failed"}}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Mode != "raw_items" || got.Status != "failed" {
		t.Fatalf("got %+v", got)
	}
}

func TestPostFollowsRedirectWithBody(t *testing.T) {
	srv := fakeAppsScript(t, func(mode string, _ url.Values, body []byte) interface{} {
		var in struct{ Sources []map[string]interface{} }
		json.Unmarshal(body, &in)
		return map[string]interface{}{"ok": true, "updated": len(in.Sources), "mode": mode}
	})
	var got struct {
		Updated int
		Mode    string
	}
	payload := map[string]interface{}{"sources": []map[string]string{{"id": "src_a"}, {"id": "src_b"}}}
	if err := newTestClient(srv, "tok").Post(context.Background(), "update_sources", payload, &got); err != nil {
		t.Fatal(err)
	}
	if got.Updated != 2 || got.Mode != "update_sources" {
		t.Fatalf("got %+v", got)
	}
}

func TestEnvelopeErrors(t *testing.T) {
	srv := fakeAppsScript(t, func(string, url.Values, []byte) interface{} { return nil })
	err := newTestClient(srv, "wrong").Get(context.Background(), "sources", nil, &struct{}{})
	if err == nil || err.Error() != "sheet: unauthorized" {
		t.Fatalf("want sheet: unauthorized, got %v", err)
	}

	html := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>Sign in</html>"))
	}))
	defer html.Close()
	err = NewClient(config.SheetConfig{URL: html.URL, Token: "tok", TimeoutSeconds: 5}).Get(context.Background(), "sources", nil, &struct{}{})
	if err == nil || !strings.Contains(err.Error(), "not JSON") {
		t.Fatalf("want not JSON error, got %v", err)
	}
}

func TestNotConfigured(t *testing.T) {
	err := NewClient(config.SheetConfig{}).Get(context.Background(), "sources", nil, &struct{}{})
	if err != ErrNotConfigured {
		t.Fatalf("want ErrNotConfigured, got %v", err)
	}
}
