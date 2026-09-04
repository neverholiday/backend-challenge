package http_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	httpadapter "github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/adapters/http"
	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/application"
	"github.com/neverholiday/backend-challenge/solutions/user-management-api/internal/domain"
)

type testServer struct {
	*httpadapter.Server
	repo *fakeUserRepository
}

func newTestServer() testServer {
	return newTestServerWithLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// newTestServerWithLogger builds a test server logging to logger, so tests can
// assert on what the logging middleware emitted.
func newTestServerWithLogger(logger *slog.Logger) testServer {
	repo := newFakeUserRepository()
	hasher := &fakePasswordHasher{}
	tokenService := &fakeTokenService{}

	server := httpadapter.NewServer(
		application.NewRegisterUser(repo, hasher),
		application.NewAuthenticateUser(repo, hasher, tokenService),
		application.NewGetUser(repo),
		application.NewListUsers(repo),
		application.NewUpdateUser(repo),
		application.NewDeleteUser(repo),
		tokenService,
		logger,
	)
	return testServer{Server: server, repo: repo}
}

func doJSON(t *testing.T, s testServer, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v, want nil", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}

	req := httptest.NewRequestWithContext(t.Context(), method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func TestServer_Healthz(t *testing.T) {
	s := newTestServer()
	rec := doJSON(t, s, "GET", "/healthz", "", nil)
	if rec.Code != 200 {
		t.Errorf("GET /healthz status = %d, want 200", rec.Code)
	}
}

func TestServer_RegisterAndLogin(t *testing.T) {
	s := newTestServer()

	t.Run("register creates a user and hides the password", func(t *testing.T) {
		rec := doJSON(t, s, "POST", "/api/v1/auth/register", "", map[string]string{
			"name":     "Jane Doe",
			"email":    "jane@example.com",
			"password": "s3cret123",
		})
		if rec.Code != 201 {
			t.Fatalf("register status = %d, want 201, body = %s", rec.Code, rec.Body.String())
		}
		if bytes.Contains(rec.Body.Bytes(), []byte("s3cret123")) {
			t.Error("register response leaked the plaintext password")
		}
	})

	t.Run("register rejects an invalid email with 400", func(t *testing.T) {
		rec := doJSON(t, s, "POST", "/api/v1/auth/register", "", map[string]string{
			"name":     "Jane Doe",
			"email":    "not-an-email",
			"password": "s3cret123",
		})
		if rec.Code != 400 {
			t.Errorf("register status = %d, want 400", rec.Code)
		}
	})

	t.Run("register rejects a duplicate email with 409", func(t *testing.T) {
		doJSON(t, s, "POST", "/api/v1/auth/register", "", map[string]string{
			"name": "Jane Doe", "email": "dup@example.com", "password": "s3cret123",
		})
		rec := doJSON(t, s, "POST", "/api/v1/auth/register", "", map[string]string{
			"name": "Jane Two", "email": "dup@example.com", "password": "s3cret123",
		})
		if rec.Code != 409 {
			t.Errorf("register status = %d, want 409", rec.Code)
		}
	})

	t.Run("login returns a token for correct credentials", func(t *testing.T) {
		doJSON(t, s, "POST", "/api/v1/auth/register", "", map[string]string{
			"name": "Bob", "email": "bob@example.com", "password": "s3cret123",
		})

		rec := doJSON(t, s, "POST", "/api/v1/auth/login", "", map[string]string{
			"email": "bob@example.com", "password": "s3cret123",
		})
		if rec.Code != 200 {
			t.Fatalf("login status = %d, want 200, body = %s", rec.Code, rec.Body.String())
		}

		var resp struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("json.Unmarshal() error = %v, want nil", err)
		}
		if resp.Token == "" {
			t.Error("login response has empty token")
		}
	})

	t.Run("login rejects a wrong password with 401", func(t *testing.T) {
		doJSON(t, s, "POST", "/api/v1/auth/register", "", map[string]string{
			"name": "Carl", "email": "carl@example.com", "password": "s3cret123",
		})
		rec := doJSON(t, s, "POST", "/api/v1/auth/login", "", map[string]string{
			"email": "carl@example.com", "password": "wrong-password",
		})
		if rec.Code != 401 {
			t.Errorf("login status = %d, want 401", rec.Code)
		}
	})
}

func TestServer_ProtectedRoutes(t *testing.T) {
	s := newTestServer()

	user := domain.User{ID: "user-1", Name: "Jane", Email: "jane@example.com", PasswordHash: "hashed:pw"}
	if err := s.repo.CreateUser(t.Context(), user); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}

	t.Run("rejects a request with no token", func(t *testing.T) {
		rec := doJSON(t, s, "GET", "/api/v1/users", "", nil)
		if rec.Code != 401 {
			t.Errorf("GET /users status = %d, want 401", rec.Code)
		}
	})

	t.Run("rejects a request with an invalid token", func(t *testing.T) {
		rec := doJSON(t, s, "GET", "/api/v1/users", "invalid", nil)
		if rec.Code != 401 {
			t.Errorf("GET /users status = %d, want 401", rec.Code)
		}
	})

	t.Run("lists users with a valid token", func(t *testing.T) {
		rec := doJSON(t, s, "GET", "/api/v1/users", user.ID, nil)
		if rec.Code != 200 {
			t.Fatalf("GET /users status = %d, want 200, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("gets a user by id", func(t *testing.T) {
		rec := doJSON(t, s, "GET", "/api/v1/users/"+user.ID, user.ID, nil)
		if rec.Code != 200 {
			t.Fatalf("GET /users/:id status = %d, want 200, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("returns 404 for an unknown user", func(t *testing.T) {
		rec := doJSON(t, s, "GET", "/api/v1/users/does-not-exist", user.ID, nil)
		if rec.Code != 404 {
			t.Errorf("GET /users/:id status = %d, want 404", rec.Code)
		}
	})

	t.Run("updates a user's name", func(t *testing.T) {
		rec := doJSON(t, s, "PATCH", "/api/v1/users/"+user.ID, user.ID, map[string]string{"name": "Janet"})
		if rec.Code != 200 {
			t.Fatalf("PATCH /users/:id status = %d, want 200, body = %s", rec.Code, rec.Body.String())
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte("Janet")) {
			t.Errorf("PATCH /users/:id body = %s, want it to contain the updated name", rec.Body.String())
		}
	})

	t.Run("rejects an update with no fields", func(t *testing.T) {
		rec := doJSON(t, s, "PATCH", "/api/v1/users/"+user.ID, user.ID, map[string]string{})
		if rec.Code != 400 {
			t.Errorf("PATCH /users/:id status = %d, want 400", rec.Code)
		}
	})

	t.Run("deletes a user", func(t *testing.T) {
		rec := doJSON(t, s, "DELETE", "/api/v1/users/"+user.ID, user.ID, nil)
		if rec.Code != 204 {
			t.Fatalf("DELETE /users/:id status = %d, want 204, body = %s", rec.Code, rec.Body.String())
		}

		rec = doJSON(t, s, "GET", "/api/v1/users/"+user.ID, user.ID, nil)
		if rec.Code != 404 {
			t.Errorf("GET /users/:id after delete status = %d, want 404", rec.Code)
		}
	})
}

func TestServer_LoggingMiddlewareRecordsErrorStatus(t *testing.T) {
	var logs bytes.Buffer
	s := newTestServerWithLogger(slog.New(slog.NewTextHandler(&logs, nil)))

	rec := doJSON(t, s, "GET", "/api/v1/does-not-exist", "", nil)
	if rec.Code != 404 {
		t.Fatalf("GET /api/v1/does-not-exist status = %d, want 404", rec.Code)
	}

	line := logs.String()
	if !strings.Contains(line, "status=404") {
		t.Errorf("log line = %q, want it to record status=404", line)
	}
	if !strings.Contains(line, "path=/api/v1/does-not-exist") {
		t.Errorf("log line = %q, want it to record the requested path", line)
	}
}

func TestServer_LoggingMiddlewareRecordsSuccessStatus(t *testing.T) {
	var logs bytes.Buffer
	s := newTestServerWithLogger(slog.New(slog.NewTextHandler(&logs, nil)))

	rec := doJSON(t, s, "GET", "/healthz", "", nil)
	if rec.Code != 200 {
		t.Fatalf("GET /healthz status = %d, want 200", rec.Code)
	}

	line := logs.String()
	for _, want := range []string{"method=GET", "path=/healthz", "status=200", "duration="} {
		if !strings.Contains(line, want) {
			t.Errorf("log line = %q, want it to contain %q", line, want)
		}
	}
}

func TestServer_RegisterRejectsOverlongPassword(t *testing.T) {
	s := newTestServer()

	rec := doJSON(t, s, "POST", "/api/v1/auth/register", "", map[string]string{
		"name":     "Jane Doe",
		"email":    "jane@example.com",
		"password": strings.Repeat("a", domain.MaxPasswordLength+1),
	})
	if rec.Code != 400 {
		t.Fatalf("POST /api/v1/auth/register status = %d, want 400", rec.Code)
	}
}
