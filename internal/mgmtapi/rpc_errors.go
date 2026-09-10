package mgmtapi

import (
	"errors"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	"shepherd/internal/auth"
)

// errInternal is the message returned to the client for mapError's default
// (unrecognized) case. The real error is never sent over the wire for that
// case — only a caller that has already logged err server-side (the
// existing convention across this package, e.g. s.logger.Warn("...", "err",
// err) before returning) has any record of what actually failed, matching
// how every other CodeInternal site in mgmtapi already behaves.
var errInternal = errors.New("internal error")

// mapError maps the store and auth packages' sentinel errors, plus common
// pgconn failure codes, onto the connect.Code an RPC handler should return.
// It exists because the mapping was previously duplicated ad hoc at ~274
// call sites across mgmtapi (rpc_destination.go alone had ~39), and several
// of them collapsed every possible store error onto one fixed code — e.g. a
// *ByID lookup helper reporting "not found" even when the real failure was
// a connection error, not a missing row. Call mapError on a raw store/auth
// error to get the right code; a caller that wants a nicer, domain-specific
// message for the "this row doesn't exist" case should still check
// errors.Is(err, pgx.ErrNoRows) itself before falling back to mapError for
// everything else (see loadOwnedDestination for the pattern).
func mapError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, auth.ErrUnauthenticated):
		return connect.NewError(connect.CodeUnauthenticated, err)
	case errors.Is(err, auth.ErrOrgNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, auth.ErrInvalidOrgID):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, auth.ErrHelmManaged),
		errors.Is(err, auth.ErrEncryptionUnavailable),
		errors.Is(err, auth.ErrNoSettings):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	case isUniqueViolation(err):
		return connect.NewError(connect.CodeAlreadyExists, err)
	case isFKViolation(err):
		return connect.NewError(connect.CodeAlreadyExists, err)
	default:
		return connect.NewError(connect.CodeInternal, errInternal)
	}
}
