package kong

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestClient spins up an httptest server that delegates every request to
// fn, and returns a Client pointed at it. The server is closed on test
// cleanup.
func newTestClient(t *testing.T, fn func(w http.ResponseWriter, r *http.Request)) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(fn))
	t.Cleanup(srv.Close)
	return &Client{baseURL: srv.URL, http: srv.Client()}
}

func TestNewClient_EmptyAdminURL_ReturnsNil(t *testing.T) {
	if c := NewClient(""); c != nil {
		t.Fatalf("expected nil client for empty admin url, got %+v", c)
	}
}

func TestNewClient_NonEmptyAdminURL_ReturnsClient(t *testing.T) {
	c := NewClient("http://localhost:8001")
	if c == nil {
		t.Fatalf("expected non-nil client for non-empty admin url")
	}
	if c.baseURL != "http://localhost:8001" {
		t.Fatalf("unexpected baseURL: %s", c.baseURL)
	}
	if c.http == nil {
		t.Fatalf("expected non-nil http client")
	}
}

// --- Service CRUD --------------------------------------------------------

func TestGetService_NotFound_ReturnsFoundFalse(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if got, want := r.URL.Path, "/services/agent-x"; got != want {
			t.Errorf("expected path %s, got %s", want, got)
		}
		w.WriteHeader(http.StatusNotFound)
	})
	svc, found, err := c.GetService(context.Background(), "agent-x")
	if err != nil || found {
		t.Fatalf("expected found=false nil err, got found=%v err=%v", found, err)
	}
	if svc != nil {
		t.Fatalf("expected nil svc on not-found, got %+v", svc)
	}
}

func TestGetService_Found_ReturnsService(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"svc-1","name":"agent-general","protocol":"http","host":"h","port":8080,"tags":["managed"]}`))
	})
	svc, found, err := c.GetService(context.Background(), "agent-general")
	if err != nil || !found {
		t.Fatalf("expected found=true nil err, got found=%v err=%v", found, err)
	}
	if svc.ID != "svc-1" || svc.Name != "agent-general" || svc.Host != "h" || svc.Port != 8080 {
		t.Fatalf("unexpected service: %+v", svc)
	}
	if len(svc.Tags) != 1 || svc.Tags[0] != "managed" {
		t.Fatalf("unexpected tags: %+v", svc.Tags)
	}
}

func TestCreateService_PostsBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if got, want := r.URL.Path, "/services"; got != want {
			t.Errorf("expected path %s, got %s", want, got)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}
		var in Service
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if in.Name != "agent-general" || in.Host != "h" || in.Port != 1 {
			t.Errorf("unexpected request body: %+v", in)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"svc-1","name":"agent-general"}`))
	})
	out, err := c.CreateService(context.Background(), &Service{Name: "agent-general", Host: "h", Port: 1})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out.ID != "svc-1" || out.Name != "agent-general" {
		t.Fatalf("unexpected response: %+v", out)
	}
}

func TestCreateService_ServerError_ReturnsError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"boom"}`))
	})
	out, err := c.CreateService(context.Background(), &Service{Name: "agent-general"})
	if err == nil {
		t.Fatalf("expected error, got nil; out=%+v", out)
	}
	if out != nil {
		t.Fatalf("expected nil out on error, got %+v", out)
	}
}

func TestUpdateService_PatchBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		if got, want := r.URL.Path, "/services/agent-x"; got != want {
			t.Errorf("expected path %s, got %s", want, got)
		}
		var in Service
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if in.Host != "new-host" || in.Port != 9090 {
			t.Errorf("unexpected body: %+v", in)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"svc-1","name":"agent-x","host":"new-host","port":9090}`))
	})
	out, err := c.UpdateService(context.Background(), "agent-x", &Service{Host: "new-host", Port: 9090})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out.ID != "svc-1" || out.Host != "new-host" || out.Port != 9090 {
		t.Fatalf("unexpected response: %+v", out)
	}
}

func TestDeleteService_NotFound_IsIdempotent(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if got, want := r.URL.Path, "/services/agent-x"; got != want {
			t.Errorf("expected path %s, got %s", want, got)
		}
		w.WriteHeader(http.StatusNotFound)
	})
	if err := c.DeleteService(context.Background(), "agent-x"); err != nil {
		t.Fatalf("idempotent delete should swallow 404, got %v", err)
	}
}

func TestDeleteService_ServerError_ReturnsError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"boom"}`))
	})
	if err := c.DeleteService(context.Background(), "agent-x"); err == nil {
		t.Fatalf("expected error from 500, got nil")
	}
}

func TestListServicesByTag_ParsesDataArray(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if got, want := r.URL.Path, "/services"; got != want {
			t.Errorf("expected path %s, got %s", want, got)
		}
		if got, want := r.URL.RawQuery, "tags=managed-by-cp"; got != want {
			t.Errorf("expected query %s, got %s", want, got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"svc-1","name":"a"},{"id":"svc-2","name":"b"}],"next":null}`))
	})
	svcs, err := c.ListServicesByTag(context.Background(), "managed-by-cp")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(svcs) != 2 {
		t.Fatalf("expected 2 services, got %d: %+v", len(svcs), svcs)
	}
	if svcs[0].ID != "svc-1" || svcs[0].Name != "a" {
		t.Fatalf("unexpected first svc: %+v", svcs[0])
	}
	if svcs[1].ID != "svc-2" || svcs[1].Name != "b" {
		t.Fatalf("unexpected second svc: %+v", svcs[1])
	}
}

func TestListServicesByTag_ServerError_ReturnsError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		io.WriteString(w, "upstream down")
	})
	if _, err := c.ListServicesByTag(context.Background(), "x"); err == nil {
		t.Fatalf("expected error from 502, got nil")
	}
}

// Kong paginates list endpoints via the offset cursor: a response carrying a
// non-empty "offset" has more pages; the last page omits it. The client must
// follow the cursor and aggregate records across pages.
func TestListServicesByTag_FollowsPaginationOffset(t *testing.T) {
	page2Requested := false
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if got, want := r.URL.Path, "/services"; got != want {
			t.Errorf("expected path %s, got %s", want, got)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("offset") == "" {
			if got, want := r.URL.RawQuery, "tags=managed-by-cp"; got != want {
				t.Errorf("expected query %s, got %s", want, got)
			}
			w.Write([]byte(`{"data":[{"id":"svc-1","name":"a"},{"id":"svc-2","name":"b"}],"offset":"cursor-1","next":"/services?tags=managed-by-cp&offset=cursor-1"}`))
			return
		}
		if got, want := r.URL.RawQuery, "tags=managed-by-cp&offset=cursor-1"; got != want {
			t.Errorf("expected query %s, got %s", want, got)
		}
		page2Requested = true
		w.Write([]byte(`{"data":[{"id":"svc-3","name":"c"}],"next":null}`))
	})
	svcs, err := c.ListServicesByTag(context.Background(), "managed-by-cp")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !page2Requested {
		t.Fatal("expected a second request carrying the offset cursor")
	}
	if len(svcs) != 3 {
		t.Fatalf("expected 3 services across both pages, got %d: %+v", len(svcs), svcs)
	}
	if svcs[0].ID != "svc-1" || svcs[1].ID != "svc-2" || svcs[2].ID != "svc-3" {
		t.Fatalf("unexpected aggregated services: %+v", svcs)
	}
}

// A server that keeps returning non-empty data with the SAME offset cursor
// (A→A, or a longer A→B→A cycle) would spin the pagination loop forever under
// a long-lived reconcile context: the empty-page defense never triggers. The
// client must detect the repeated cursor and return an error. The handler
// caps the number of requests it will serve so a regression to an unguarded
// loop fails this test fast instead of hanging it.
func TestListServicesByTag_RepeatedCursorCycleReturnsError(t *testing.T) {
	const maxRequests = 8
	served := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		served++
		if served > maxRequests {
			t.Errorf("pagination loop is spinning: served %d requests, cap %d", served, maxRequests)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"svc-1","name":"a"}],"offset":"cursor-1","next":"/services?tags=managed-by-cp&offset=cursor-1"}`))
	})
	if _, err := c.ListServicesByTag(context.Background(), "managed-by-cp"); err == nil {
		t.Fatal("expected error on repeated pagination cursor, got nil")
	}
}

// --- Route CRUD ----------------------------------------------------------

func TestGetRoute_NotFound_ReturnsFoundFalse(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if got, want := r.URL.Path, "/routes/agent-x"; got != want {
			t.Errorf("expected path %s, got %s", want, got)
		}
		w.WriteHeader(http.StatusNotFound)
	})
	route, found, err := c.GetRoute(context.Background(), "agent-x")
	if err != nil || found {
		t.Fatalf("expected found=false nil err, got found=%v err=%v", found, err)
	}
	if route != nil {
		t.Fatalf("expected nil route, got %+v", route)
	}
}

func TestGetRoute_Found_ReturnsRoute(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"r-1","name":"agent-x","hosts":["a.example.com"],"paths":["/x"],"strip_path":true,"service":{"id":"svc-1"}}`))
	})
	route, found, err := c.GetRoute(context.Background(), "agent-x")
	if err != nil || !found {
		t.Fatalf("expected found=true nil err, got found=%v err=%v", found, err)
	}
	if route.ID != "r-1" || len(route.Hosts) != 1 || route.Hosts[0] != "a.example.com" {
		t.Fatalf("unexpected route: %+v", route)
	}
	if len(route.Paths) != 1 || route.Paths[0] != "/x" {
		t.Fatalf("unexpected paths: %+v", route.Paths)
	}
	if !route.StripPath {
		t.Fatalf("expected strip_path=true")
	}
	if route.Service == nil || route.Service.ID != "svc-1" {
		t.Fatalf("unexpected service ref: %+v", route.Service)
	}
}

func TestCreateRoute_PostsBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if got, want := r.URL.Path, "/routes"; got != want {
			t.Errorf("expected path %s, got %s", want, got)
		}
		var in Route
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if in.Name != "r-1" {
			t.Errorf("unexpected name: %s", in.Name)
		}
		if len(in.Hosts) != 1 || in.Hosts[0] != "a.example.com" {
			t.Errorf("unexpected hosts: %+v", in.Hosts)
		}
		if len(in.Paths) != 1 || in.Paths[0] != "/x" {
			t.Errorf("unexpected paths: %+v", in.Paths)
		}
		if !in.StripPath {
			t.Errorf("expected strip_path=true")
		}
		if in.Service == nil || in.Service.ID != "svc-1" {
			t.Errorf("unexpected service ref: %+v", in.Service)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"r-1","name":"r-1","hosts":["a.example.com"],"paths":["/x"],"strip_path":true,"service":{"id":"svc-1"}}`))
	})
	out, err := c.CreateRoute(context.Background(), &Route{
		Name:      "r-1",
		Hosts:     []string{"a.example.com"},
		Paths:     []string{"/x"},
		StripPath: true,
		Service:   &ServiceRef{ID: "svc-1"},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out.ID != "r-1" || out.Service == nil || out.Service.ID != "svc-1" {
		t.Fatalf("unexpected response: %+v", out)
	}
}

func TestCreateRoute_ServerError_ReturnsError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"message":"bad fields"}`))
	})
	if _, err := c.CreateRoute(context.Background(), &Route{Name: "r-1"}); err == nil {
		t.Fatalf("expected error from 400, got nil")
	}
}

func TestDeleteRoute_NotFound_IsIdempotent(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if got, want := r.URL.Path, "/routes/agent-x"; got != want {
			t.Errorf("expected path %s, got %s", want, got)
		}
		w.WriteHeader(http.StatusNotFound)
	})
	if err := c.DeleteRoute(context.Background(), "agent-x"); err != nil {
		t.Fatalf("idempotent delete should swallow 404, got %v", err)
	}
}

func TestDeleteRoute_ServerError_ReturnsError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"boom"}`))
	})
	if err := c.DeleteRoute(context.Background(), "agent-x"); err == nil {
		t.Fatalf("expected error from 500, got nil")
	}
}

func TestListRoutesByTag_ParsesDataArray(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if got, want := r.URL.Path, "/routes"; got != want {
			t.Errorf("expected path %s, got %s", want, got)
		}
		if got, want := r.URL.RawQuery, "tags=managed-by-cp"; got != want {
			t.Errorf("expected query %s, got %s", want, got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"r-1","name":"agent-x-route","paths":["/x"],"strip_path":true,"service":{"id":"svc-1"},"tags":["managed-by-cp"]},{"id":"r-2","name":"agent-y-route","hosts":["a.example.com"],"paths":["/y"],"strip_path":false}],"next":null}`))
	})
	routes, err := c.ListRoutesByTag(context.Background(), "managed-by-cp")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d: %+v", len(routes), routes)
	}
	if routes[0].ID != "r-1" || routes[0].Name != "agent-x-route" {
		t.Fatalf("unexpected first route: %+v", routes[0])
	}
	if len(routes[0].Paths) != 1 || routes[0].Paths[0] != "/x" {
		t.Fatalf("unexpected first route paths: %+v", routes[0].Paths)
	}
	if !routes[0].StripPath {
		t.Fatalf("expected strip_path=true on first route")
	}
	if routes[0].Service == nil || routes[0].Service.ID != "svc-1" {
		t.Fatalf("unexpected first route service ref: %+v", routes[0].Service)
	}
	if len(routes[0].Tags) != 1 || routes[0].Tags[0] != "managed-by-cp" {
		t.Fatalf("unexpected first route tags: %+v", routes[0].Tags)
	}
	if routes[1].ID != "r-2" || routes[1].Name != "agent-y-route" {
		t.Fatalf("unexpected second route: %+v", routes[1])
	}
	if len(routes[1].Hosts) != 1 || routes[1].Hosts[0] != "a.example.com" {
		t.Fatalf("unexpected second route hosts: %+v", routes[1].Hosts)
	}
	if routes[1].StripPath {
		t.Fatalf("expected strip_path=false on second route")
	}
}

func TestListRoutesByTag_ServerError_ReturnsError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		io.WriteString(w, "upstream down")
	})
	if _, err := c.ListRoutesByTag(context.Background(), "x"); err == nil {
		t.Fatalf("expected error from 502, got nil")
	}
}

// Same pagination contract as services: follow the offset cursor until the
// response omits it, aggregating routes from every page.
func TestListRoutesByTag_FollowsPaginationOffset(t *testing.T) {
	page2Requested := false
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if got, want := r.URL.Path, "/routes"; got != want {
			t.Errorf("expected path %s, got %s", want, got)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("offset") == "" {
			if got, want := r.URL.RawQuery, "tags=managed-by-cp"; got != want {
				t.Errorf("expected query %s, got %s", want, got)
			}
			w.Write([]byte(`{"data":[{"id":"r-1","name":"agent-x-route","paths":["/x"],"strip_path":true,"service":{"id":"svc-1"},"tags":["managed-by-cp"]}],"offset":"cursor-1","next":"/routes?tags=managed-by-cp&offset=cursor-1"}`))
			return
		}
		if got, want := r.URL.RawQuery, "tags=managed-by-cp&offset=cursor-1"; got != want {
			t.Errorf("expected query %s, got %s", want, got)
		}
		page2Requested = true
		w.Write([]byte(`{"data":[{"id":"r-2","name":"agent-y-route","hosts":["a.example.com"],"paths":["/y"],"strip_path":false}],"next":null}`))
	})
	routes, err := c.ListRoutesByTag(context.Background(), "managed-by-cp")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !page2Requested {
		t.Fatal("expected a second request carrying the offset cursor")
	}
	if len(routes) != 2 {
		t.Fatalf("expected 2 routes across both pages, got %d: %+v", len(routes), routes)
	}
	if routes[0].ID != "r-1" || routes[0].Name != "agent-x-route" {
		t.Fatalf("unexpected first route: %+v", routes[0])
	}
	if routes[1].ID != "r-2" || routes[1].Name != "agent-y-route" {
		t.Fatalf("unexpected second route: %+v", routes[1])
	}
	if len(routes[1].Paths) != 1 || routes[1].Paths[0] != "/y" {
		t.Fatalf("unexpected second route paths: %+v", routes[1].Paths)
	}
}

// Same contract as the services variant: a server that repeats a non-empty
// offset cursor alongside non-empty data must produce an error, not an
// infinite loop. The request cap keeps a regression fail-fast.
func TestListRoutesByTag_RepeatedCursorCycleReturnsError(t *testing.T) {
	const maxRequests = 8
	served := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		served++
		if served > maxRequests {
			t.Errorf("pagination loop is spinning: served %d requests, cap %d", served, maxRequests)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"r-1","name":"agent-x-route","paths":["/x"],"strip_path":true,"service":{"id":"svc-1"}},{"id":"r-2","name":"agent-y-route","hosts":["a.example.com"],"paths":["/y"],"strip_path":false}],"offset":"cursor-1","next":"/routes?tags=managed-by-cp&offset=cursor-1"}`))
	})
	if _, err := c.ListRoutesByTag(context.Background(), "managed-by-cp"); err == nil {
		t.Fatal("expected error on repeated pagination cursor, got nil")
	}
}
