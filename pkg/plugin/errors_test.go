package plugin

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestUserErrorMessage_TransportErrors pins that each transport failure category maps to a fixed
// message that carries none of the URL, host, IP or port details of the underlying error.
func TestUserErrorMessage_TransportErrors(t *testing.T) {
	const leak = "10.1.2.3:8080"
	wrap := func(err error) error {
		return &url.Error{Op: "Get", URL: "http://" + leak + "/jkql", Err: err}
	}
	opErr := func(err error) error {
		return &net.OpError{Op: "dial", Net: "tcp", Addr: &net.TCPAddr{IP: net.ParseIP("10.1.2.3"), Port: 8080}, Err: err}
	}

	tests := []struct {
		name string
		err  error
		want string
	}{
		{"dns", wrap(opErr(&net.DNSError{Err: "no such host", Name: "internal.example", Server: leak})), "could not resolve"},
		{"tls", wrap(&tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}), "TLS handshake"},
		{"refused", wrap(opErr(os.NewSyscallError("connect", syscall.ECONNREFUSED))), "was refused"},
		{"timeout", wrap(context.DeadlineExceeded), "timed out"},
		// The shape the Grafana SDK HTTP client returns: its wrapper sits between *url.Error and the
		// dial error and has no Timeout method, so *url.Error.Timeout() reports false.
		{"dial timeout behind a wrapper", wrap(sdkWrapper{opErr(os.ErrDeadlineExceeded)}), "timed out"},
		{"canceled", wrap(context.Canceled), "canceled"},
		{"redirect", wrap(errCrossHostRedirect), "redirected"},
		{"other dial error", wrap(opErr(errors.New("network is unreachable " + leak))), "could not reach"},
		{"read error", opErr(errors.New("connection reset by peer")), "could not reach"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := userErrorMessage(tt.err, targetDataService)
			if !strings.Contains(got, tt.want) {
				t.Errorf("message = %q, want it to contain %q", got, tt.want)
			}
			if strings.Contains(got, "10.1.2.3") || strings.Contains(got, "8080") || strings.Contains(got, "internal.example") {
				t.Errorf("message leaks address details: %q", got)
			}
		})
	}
}

// TestBuildCustomErrorMessage_AccessToken pins that both server wordings of the token error are
// replaced, so the token is never echoed back to the user.
func TestBuildCustomErrorMessage_AccessToken(t *testing.T) {
	for _, msg := range []string{
		"com.nastel.jkool.jkql.admin.JKQLAuthenticationException: Undefined or expired access token 'secret-1', item: Parameter, stmt: Get Params",
		"com.nastel.jkool.jkql.admin.JKQLAuthenticationException: Undefined, invalid, or expired access token 'secret-1', item: Parameter, stmt: Get Params",
	} {
		if got := buildCustomErrorMessage(msg); got != "The access token is undefined, invalid or expired" {
			t.Errorf("buildCustomErrorMessage(%q) = %q", msg, got)
		}
	}
	if got := buildCustomErrorMessage("Syntax error at line 1"); got != "Syntax error at line 1" {
		t.Errorf("other messages must pass through, got %q", got)
	}
}

// sdkWrapper stands in for the Grafana SDK's status.ErrorWithSource: it wraps an error but has no
// Timeout method.
type sdkWrapper struct{ err error }

func (w sdkWrapper) Error() string { return w.err.Error() }
func (w sdkWrapper) Unwrap() error { return w.err }

// TestUserErrorMessage_KeepsOtherErrors pins that jKQL errors, status errors and the plugin's own
// messages pass through unchanged, and that the size error names the service.
func TestUserErrorMessage_KeepsOtherErrors(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{&queryError{"Syntax error at line 1"}, "Syntax error at line 1"},
		{errors.New("service URL is required"), "service URL is required"},
		{&statusError{target: targetDataService, statusCode: 401, hint: "check the access token"},
			"meshIQ Platform service returned HTTP 401; check the access token"},
		{&statusError{target: targetCompletionService, statusCode: 502, nonJSON: true},
			"meshIQ jKQL completion service returned a non-JSON response (HTTP 502)"},
		{errResponseTooLarge, "the response from the meshIQ Platform service exceeded the 100 MB limit"},
	}
	for _, tt := range tests {
		if got := userErrorMessage(tt.err, targetDataService); got != tt.want {
			t.Errorf("userErrorMessage(%v) = %q, want %q", tt.err, got, tt.want)
		}
	}
}

// TestQueryDataService_StatusErrors pins the hints the dataservice client attaches.
func TestQueryDataService_StatusErrors(t *testing.T) {
	tests := []struct {
		status int
		body   string
		want   string
	}{
		{http.StatusUnauthorized, `{}`, "meshIQ Platform service returned HTTP 401; check the access token"},
		{http.StatusBadGateway, `{}`, "meshIQ Platform service returned HTTP 502"},
		{http.StatusNotFound, `<html>`, "meshIQ Platform service returned a non-JSON response (HTTP 404); check the Service URL and access token"},
	}
	for _, tt := range tests {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tt.status)
			_, _ = w.Write([]byte(tt.body))
		}))
		_, err := testDataService(context.Background(), server.Client(), MeshIqDataSourceOptions{ServiceUrl: server.URL, Token: "t"})
		server.Close()
		if got := userErrorMessage(err, targetDataService); got != tt.want {
			t.Errorf("HTTP %d: message = %q, want %q", tt.status, got, tt.want)
		}
	}
}

// TestCheckHealth_DoesNotLeakDialDetails runs Save & test against a closed local port and checks
// the message names the failure without the address.
func TestCheckHealth_DoesNotLeakDialDetails(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	addr := server.Listener.Addr().String()
	server.Close() // nothing listens on addr any more

	client := &http.Client{Timeout: 5 * time.Second}
	_, err := testDataService(context.Background(), client, MeshIqDataSourceOptions{ServiceUrl: "http://" + addr, Token: "t"})
	if err == nil {
		t.Fatal("expected a connection error")
	}
	result := newHealthError(context.Background(), err, targetDataService)
	if strings.Contains(result.Message, addr) || strings.Contains(result.Message, "127.0.0.1") {
		t.Errorf("health message leaks the address: %q", result.Message)
	}
	if !strings.Contains(result.Message, "refused") {
		t.Errorf("health message = %q, want the connection-refused message", result.Message)
	}
}
