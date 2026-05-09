package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func setupTestServer(t *testing.T) http.Handler {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := newStore(dbPath)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}
	t.Cleanup(func() { s.db.Close() })
	return newRouter(s)
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	return rr
}

func decode[T any](t *testing.T, rr *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rr.Body).Decode(&v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}

func TestHealth(t *testing.T) {
	h := setupTestServer(t)
	rr := do(t, h, "GET", "/health", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	got := decode[map[string]string](t, rr)
	if got["status"] != "ok" {
		t.Fatalf("status field = %q, want ok", got["status"])
	}
}

func TestItemsCRUD(t *testing.T) {
	h := setupTestServer(t)

	rr := do(t, h, "POST", "/items", `{"name":"first"}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", rr.Code)
	}
	created := decode[Item](t, rr)
	if created.ID == 0 || created.Name != "first" {
		t.Fatalf("created = %+v", created)
	}

	rr = do(t, h, "GET", "/items", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", rr.Code)
	}
	items := decode[[]Item](t, rr)
	if len(items) != 1 || items[0] != created {
		t.Fatalf("list = %+v", items)
	}

	path := "/items/" + strconv.FormatInt(created.ID, 10)
	rr = do(t, h, "GET", path, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200", rr.Code)
	}
	if got := decode[Item](t, rr); got != created {
		t.Fatalf("get = %+v, want %+v", got, created)
	}

	rr = do(t, h, "PUT", path, `{"name":"renamed"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200", rr.Code)
	}
	if got := decode[Item](t, rr); got.Name != "renamed" {
		t.Fatalf("update name = %q, want renamed", got.Name)
	}

	rr = do(t, h, "DELETE", path, "")
	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", rr.Code)
	}

	rr = do(t, h, "GET", path, "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("get-after-delete status = %d, want 404", rr.Code)
	}
}

func TestCreateValidation(t *testing.T) {
	h := setupTestServer(t)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"invalid json", `{`, http.StatusBadRequest},
		{"missing name", `{}`, http.StatusBadRequest},
		{"empty name", `{"name":""}`, http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rr := do(t, h, "POST", "/items", c.body)
			if rr.Code != c.want {
				t.Fatalf("status = %d, want %d", rr.Code, c.want)
			}
		})
	}
}

func TestItemNotFound(t *testing.T) {
	h := setupTestServer(t)

	cases := []struct {
		method, path, body string
	}{
		{"GET", "/items/999", ""},
		{"PUT", "/items/999", `{"name":"x"}`},
		{"DELETE", "/items/999", ""},
	}
	for _, c := range cases {
		t.Run(c.method, func(t *testing.T) {
			rr := do(t, h, c.method, c.path, c.body)
			if rr.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", rr.Code)
			}
		})
	}
}

func TestInvalidID(t *testing.T) {
	h := setupTestServer(t)
	rr := do(t, h, "GET", "/items/abc", "")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

