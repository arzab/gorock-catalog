package rockhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	rh "github.com/arzab/gorock-kit/rockhandlers"
)

// --- params and responses ---

type CreateParams struct {
	Authorization string   `header:"Authorization" json:"-"`
	ShopID        int      `path:"shop"`
	DryRun        bool     `query:"dry_run"`
	Tags          []string `query:"tag"`
	Item          string   `json:"item"`
	Qty           int      `json:"qty"`
}

func (p *CreateParams) Validate() error {
	if p.Qty <= 0 {
		return rh.Invalid("invalid order").WithFields(map[string]string{"qty": "must be > 0"})
	}
	return nil
}

type CreateResponse struct {
	Shop   int      `json:"shop"`
	Item   string   `json:"item"`
	DryRun bool     `json:"dry_run"`
	Tags   []string `json:"tags"`
	Owner  string   `json:"owner"`
}

func (r *CreateResponse) Normalize() error { return nil }

type userKey struct{}

func checkAuth(c *rh.Ctx, p *CreateParams, _ *CreateResponse) error {
	token := strings.TrimPrefix(p.Authorization, "Bearer ")
	if token == "" {
		return rh.Unauthorized("token required")
	}
	c.Set(userKey{}, token)
	return c.Next()
}

func create(c *rh.Ctx, p *CreateParams, r *CreateResponse) error {
	owner, _ := c.Get(userKey{})
	*r = CreateResponse{Shop: p.ShopID, Item: p.Item, DryRun: p.DryRun, Tags: p.Tags, Owner: owner.(string)}
	return nil
}

type IDParams struct {
	ID int `path:"id"`
}

func (*IDParams) Validate() error { return nil }

type Empty struct{}

func (*Empty) Normalize() error { return nil }

// --- helpers ---

func handler(t *testing.T, a *App) http.Handler {
	t.Helper()
	if a.cfg.Port == "" {
		a.cfg.Port = "0"
	}
	if err := a.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return a.Handler()
}

func do(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func jsonReq(method, url, body string) *http.Request {
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func decodeBody[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}

func idChain(fn func(c *rh.Ctx, p *IDParams, r *Empty) error) *rh.Pipeline { return rh.Chain(fn) }

// --- tests ---

func TestFullChain(t *testing.T) {
	var log []string
	logMW := func(c *rh.Ctx, _ rh.Params, _ rh.Response) error {
		err := c.Next()
		log = append(log, rh.KindOf(err).String())
		return err
	}
	h := handler(t, New(Config{Prefix: "/api/"},
		Endpoint{Method: "POST", Path: "/shops/{shop}/orders", Handler: rh.Chain(checkAuth, create), Status: http.StatusCreated},
	).Use(logMW))

	req := jsonReq("POST", "/api/shops/7/orders?dry_run=true&tag=a&tag=b", `{"item":"tea","qty":2}`)
	req.Header.Set("Authorization", "Bearer ann")
	rec := do(h, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	got := decodeBody[CreateResponse](t, rec)
	if got.Shop != 7 || got.Item != "tea" || !got.DryRun || len(got.Tags) != 2 || got.Owner != "ann" {
		t.Fatalf("got %+v", got)
	}

	rec = do(h, jsonReq("POST", "/api/shops/7/orders", `{"item":"tea","qty":2}`))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: status %d", rec.Code)
	}

	req = jsonReq("POST", "/api/shops/7/orders", `{"item":"tea","qty":0}`)
	req.Header.Set("Authorization", "Bearer ann")
	rec = do(h, req)
	body := decodeBody[ErrorBody](t, rec)
	if rec.Code != 400 || body.Kind != "invalid" || body.Fields["qty"] != "must be > 0" {
		t.Fatalf("status %d, body %+v", rec.Code, body)
	}

	if got := strings.Join(log, " "); got != "none unauthorized invalid" {
		t.Fatalf("middleware log: %s", got)
	}
}

func TestErrors(t *testing.T) {
	h := handler(t, New(Config{},
		Endpoint{Method: "GET", Path: "/missing/{id}", Handler: idChain(func(_ *rh.Ctx, p *IDParams, _ *Empty) error {
			return rh.NotFound("order %d not found", p.ID)
		})},
		Endpoint{Method: "GET", Path: "/boom", Handler: idChain(func(*rh.Ctx, *IDParams, *Empty) error {
			return errors.New("db password leaked")
		})},
		Endpoint{Method: "GET", Path: "/panic", Handler: idChain(func(*rh.Ctx, *IDParams, *Empty) error { panic("oops") })},
		Endpoint{Method: "GET", Path: "/slow", Handler: idChain(func(c *rh.Ctx, _ *IDParams, _ *Empty) error {
			<-c.Done()
			return c.Err()
		}).Use(rh.Timeout(10 * time.Millisecond))},
		Endpoint{Method: "GET", Path: "/todo", Handler: rh.Chain[*IDParams, *Empty]()},
		Endpoint{Method: "DELETE", Path: "/orders/{id}", Handler: idChain(func(*rh.Ctx, *IDParams, *Empty) error { return nil })},
	))

	cases := []struct {
		url       string
		code      int
		kind, msg string
	}{
		{"/missing/42", 404, "not_found", "order 42 not found"},
		{"/missing/x", 400, "invalid", "invalid path"},
		{"/boom", 500, "internal", "internal error"},
		{"/panic", 500, "internal", "internal error"},
		{"/slow", 504, "timeout", "Gateway Timeout"},
		{"/todo", 501, "not_implemented", "Not Implemented"},
	}
	for _, c := range cases {
		rec := do(h, httptest.NewRequest("GET", c.url, nil))
		body := decodeBody[ErrorBody](t, rec)
		if rec.Code != c.code || body.Kind != c.kind || body.Message != c.msg {
			t.Fatalf("%s: status %d, body %+v", c.url, rec.Code, body)
		}
	}

	// empty response struct → 204
	if rec := do(h, httptest.NewRequest("DELETE", "/orders/1", nil)); rec.Code != 204 || rec.Body.Len() != 0 {
		t.Fatalf("delete: %d %q", rec.Code, rec.Body)
	}
	// method not allowed — net/http itself
	if rec := do(h, httptest.NewRequest("GET", "/orders/1", nil)); rec.Code != 405 {
		t.Fatalf("405: %d", rec.Code)
	}
	// client went away → 499, no body
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rec := do(h, httptest.NewRequest("GET", "/slow", nil).WithContext(ctx))
	if rec.Code != StatusClientClosedRequest || rec.Body.Len() != 0 {
		t.Fatalf("canceled: %d %q", rec.Code, rec.Body)
	}
}

// Decoding errors reach the client without Go type or struct names.
func TestDecodingErrorsAreSafe(t *testing.T) {
	h := handler(t, New(Config{}, Endpoint{Method: "POST", Path: "/shops/{shop}/orders", Handler: rh.Chain(checkAuth, create)}))
	cases := []struct {
		name, url, body string
		msg             string
		field, want     string
	}{
		{"broken JSON", "/shops/1/orders", `{"qty":`, "invalid JSON body", "", ""},
		{"wrong JSON type", "/shops/1/orders", `{"item":"tea","qty":"two"}`, "invalid JSON body", "qty", "must be a number"},
		{"wrong query type", "/shops/1/orders?dry_run=maybe", `{"item":"tea","qty":1}`, "invalid query", "dry_run", "must be true or false"},
		{"wrong path type", "/shops/x/orders", `{"item":"tea","qty":1}`, "invalid path", "shop", "must be an integer"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := jsonReq("POST", c.url, c.body)
			req.Header.Set("Authorization", "Bearer ann")
			rec := do(h, req)
			body := decodeBody[ErrorBody](t, rec)
			if rec.Code != 400 || body.Message != c.msg {
				t.Fatalf("status %d, body %+v", rec.Code, body)
			}
			if c.field != "" && body.Fields[c.field] != c.want {
				t.Fatalf("fields %v", body.Fields)
			}
			if raw := rec.Body.String(); strings.Contains(raw, "Go struct") || strings.Contains(raw, "CreateParams") {
				t.Fatalf("Go names leaked: %s", raw)
			}
		})
	}
}

// --- awkward cases: upload, download, redirect + cookie ---

type UploadParams struct {
	Title string                  `form:"title"`
	File  *multipart.FileHeader   `form:"file"`
	More  []*multipart.FileHeader `form:"more"`
}

func (p *UploadParams) Validate() error {
	if p.File == nil {
		return rh.Invalid("file is required")
	}
	return nil
}

type UploadResponse struct {
	Title, Name, Body string
	More              int
}

func (*UploadResponse) Normalize() error { return nil }

func upload(_ *rh.Ctx, p *UploadParams, r *UploadResponse) error {
	f, err := p.File.Open()
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	b, _ := io.ReadAll(f)
	*r = UploadResponse{Title: p.Title, Name: p.File.Filename, Body: string(b), More: len(p.More)}
	return nil
}

func TestUpload(t *testing.T) {
	h := handler(t, New(Config{}, Endpoint{Method: "POST", Path: "/files", Handler: rh.Chain(upload)}))

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("title", "report")
	fw, _ := mw.CreateFormFile("file", "a.txt")
	_, _ = fw.Write([]byte("hello"))
	for _, n := range []string{"b.txt", "c.txt"} {
		fw, _ = mw.CreateFormFile("more", n)
		_, _ = fw.Write([]byte(n))
	}
	_ = mw.Close()

	req := httptest.NewRequest("POST", "/files", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := do(h, req)
	got := decodeBody[UploadResponse](t, rec)
	if rec.Code != 200 || got.Title != "report" || got.Name != "a.txt" || got.Body != "hello" || got.More != 2 {
		t.Fatalf("status %d, got %+v", rec.Code, got)
	}
}

func TestBodyLimit(t *testing.T) {
	h := handler(t, New(Config{MaxBodyBytes: 10}, Endpoint{Method: "POST", Path: "/shops/{shop}/orders", Handler: rh.Chain(checkAuth, create)}))
	rec := do(h, jsonReq("POST", "/shops/1/orders", `{"item":"`+strings.Repeat("x", 100)+`","qty":1}`))
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "larger than 10 bytes") {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
}

func TestDownload(t *testing.T) {
	h := handler(t, New(Config{}, Endpoint{Method: "GET", Path: "/files/{id}", Handler: rh.Chain(
		func(_ *rh.Ctx, _ *IDParams, r *File) error {
			*r = File{Name: "report.csv", Content: strings.NewReader("a,b\n1,2\n"), Download: true}
			return nil
		})}))

	rec := do(h, httptest.NewRequest("GET", "/files/1", nil))
	if rec.Code != 200 || rec.Body.String() != "a,b\n1,2\n" || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/csv") ||
		rec.Header().Get("Content-Disposition") != "attachment; filename=report.csv" {
		t.Fatalf("status %d, headers %v, body %q", rec.Code, rec.Header(), rec.Body)
	}
	req := httptest.NewRequest("GET", "/files/1", nil)
	req.Header.Set("Range", "bytes=0-2")
	if rec = do(h, req); rec.Code != http.StatusPartialContent || rec.Body.String() != "a,b" {
		t.Fatalf("range: %d %q", rec.Code, rec.Body)
	}
}

type LoginParams struct {
	User string `form:"user"`
}

func (*LoginParams) Validate() error { return nil }

func TestRedirectAndCookie(t *testing.T) {
	h := handler(t, New(Config{}, Endpoint{Method: "POST", Path: "/login", Handler: rh.Chain(
		func(c *rh.Ctx, p *LoginParams, r *Redirect) error {
			if p.User == "" {
				return rh.Unauthorized("who are you?")
			}
			SetCookie(c, &http.Cookie{Name: "session", Value: "s-" + p.User, HttpOnly: true})
			SetHeader(c, "X-User", p.User)
			*r = Redirect{URL: "/home", Code: http.StatusSeeOther}
			return nil
		})}))

	req := httptest.NewRequest("POST", "/login", strings.NewReader("user=ann"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := do(h, req)
	if rec.Code != 303 || rec.Header().Get("Location") != "/home" || rec.Header().Get("X-User") != "ann" {
		t.Fatalf("status %d, headers %v", rec.Code, rec.Header())
	}
	if c := rec.Result().Cookies(); len(c) != 1 || c[0].Value != "s-ann" || !c[0].HttpOnly {
		t.Fatalf("cookies %v", c)
	}
}

func TestHTTPMiddleware(t *testing.T) {
	cors := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			next.ServeHTTP(w, r)
		})
	}
	h := handler(t, New(Config{Middleware: []func(http.Handler) http.Handler{cors}},
		Endpoint{Method: "DELETE", Path: "/x/{id}", Handler: idChain(func(*rh.Ctx, *IDParams, *Empty) error { return nil })}))
	if rec := do(h, httptest.NewRequest("DELETE", "/x/1", nil)); rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("headers %v", rec.Header())
	}
}

func TestInitErrors(t *testing.T) {
	ok := idChain(func(*rh.Ctx, *IDParams, *Empty) error { return nil })
	cases := map[string]*App{
		"no port":    New(Config{}, Endpoint{Method: "GET", Path: "/a", Handler: ok}),
		"no handler": New(Config{Port: "0"}, Endpoint{Method: "GET", Path: "/a"}),
		"no method":  New(Config{Port: "0"}, Endpoint{Path: "/a", Handler: ok}),
		"duplicate":  New(Config{Port: "0"}, Endpoint{Method: "GET", Path: "/a", Handler: ok}, Endpoint{Method: "GET", Path: "/a", Handler: ok}),
	}
	for name, a := range cases {
		if err := a.Init(context.Background()); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestExecAndStop(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	_ = ln.Close()

	a := New(Config{Port: port, ShutdownTimeout: time.Second},
		Endpoint{Method: "GET", Path: "/ping/{id}", Handler: idChain(func(*rh.Ctx, *IDParams, *Empty) error { return nil })})
	if err := a.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Exec(ctx) }()

	var resp *http.Response
	for range 50 {
		if resp, err = http.Get("http://127.0.0.1:" + port + "/ping/1"); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("server did not start: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Exec = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Exec did not return after cancel")
	}
	if errs := a.Stop(); errs != nil {
		t.Fatalf("Stop = %v", errs)
	}
}
