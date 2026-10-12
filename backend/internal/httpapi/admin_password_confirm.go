package httpapi

import (
	"errors"
	"net/http"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/auth"
)

// confirmAdminPassword asks for the admin's password again before a change that cannot be undone from the page (a secret, a new machine).
// It writes the problem answer itself and returns false when the password is missing, wrong or the admin is locked out.
func confirmAdminPassword(response http.ResponseWriter, request *http.Request, adminAuth AdminAuthenticator, admin auth.AuthenticatedAdmin, password string) bool {
	if len(password) < 1 || len(password) > auth.MaximumAdminPasswordBytes {
		writeProblem(response, request, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Admin password is required.", false)
		return false
	}
	switch err := adminAuth.ConfirmPassword(request.Context(), admin, password); {
	case err == nil:
		return true
	case errors.Is(err, auth.ErrLoginLocked):
		writeProblem(response, request, http.StatusTooManyRequests, "PASSWORD_CONFIRMATION_LOCKED", "Too many wrong passwords. Try again later.", true)
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeProblem(response, request, http.StatusForbidden, "PASSWORD_CONFIRMATION_FAILED", "The admin password is not correct.", false)
	default:
		writeProblem(response, request, http.StatusInternalServerError, "INTERNAL_ERROR", "Unable to confirm the password.", true)
	}
	return false
}
