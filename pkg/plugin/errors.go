package plugin

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strings"
	"syscall"
)

// buildCustomErrorMessage normalizes known dataservice error messages. The token error is
// "JKQLAuthenticationException: Undefined or expired access token '<token>', …" on v11 and
// "… Undefined, invalid, or expired access token '<token>', …" on v12; both echo the token
// back, so it is replaced with a fixed message.
func buildCustomErrorMessage(errorMessage string) string {
	if strings.Contains(errorMessage, "expired access token") {
		return "The access token is undefined, invalid or expired"
	}
	return errorMessage
}

// Service names used in user-facing error messages.
const (
	targetDataService       = "meshIQ Platform service"
	targetCompletionService = "meshIQ jKQL completion service"
)

// userErrorMessage returns a message for err that is safe to show to the user. Transport errors
// carry the request URL, IPs and ports; since the service URL is user-configurable, showing them
// would let a user probe internal addresses. They become a fixed message per failure kind, and
// the caller logs the full error. Other errors are the plugin's own or jKQL errors and are kept.
func userErrorMessage(err error, target string) string {
	var dnsErr *net.DNSError
	var netErr net.Error
	switch {
	case err == nil:
		return ""
	case errors.Is(err, errResponseTooLarge):
		return fmt.Sprintf("the response from the %s exceeded the %d MB limit", target, maxResponseBodyBytes/(1024*1024))
	case errors.Is(err, context.Canceled):
		return "the request was canceled"
	case errors.Is(err, errCrossHostRedirect):
		return fmt.Sprintf("%s redirected to a different host or scheme, which is not followed; check the URL", target)
	case errors.As(err, &dnsErr):
		return fmt.Sprintf("could not resolve the %s host name; check the URL", target)
	case isTLSError(err):
		return fmt.Sprintf("TLS handshake with the %s failed; check the URL and the server certificate", target)
	case errors.Is(err, syscall.ECONNREFUSED):
		return fmt.Sprintf("connection to the %s was refused; check the URL and that the service is running", target)
	case isTimeout(err):
		return fmt.Sprintf("the request to the %s timed out", target)
	case errors.As(err, &netErr):
		return fmt.Sprintf("could not reach the %s; check the URL", target)
	default:
		return err.Error()
	}
}

// isTimeout reports whether any error in the chain is a timeout. errors.As would stop at the
// outer *url.Error, which reports false when the Grafana SDK client has wrapped the dial error.
func isTimeout(err error) bool {
	for e := err; e != nil; e = errors.Unwrap(e) {
		if ne, ok := e.(net.Error); ok && ne.Timeout() {
			return true
		}
	}
	return false
}

// isTLSError reports whether err comes from the TLS handshake or certificate verification.
func isTLSError(err error) bool {
	var recordErr tls.RecordHeaderError
	var alertErr tls.AlertError
	var verifyErr *tls.CertificateVerificationError
	return errors.As(err, &recordErr) || errors.As(err, &alertErr) || errors.As(err, &verifyErr)
}

// statusError is a service answering with an HTTP error status or with a body that is not JSON.
type statusError struct {
	target     string
	statusCode int
	nonJSON    bool
	hint       string // optional advice for the user, e.g. "check the access token"
}

func (e *statusError) Error() string {
	msg := fmt.Sprintf("%s returned HTTP %d", e.target, e.statusCode)
	if e.nonJSON {
		msg = fmt.Sprintf("%s returned a non-JSON response (HTTP %d)", e.target, e.statusCode)
	}
	if e.hint != "" {
		msg += "; " + e.hint
	}
	return msg
}
