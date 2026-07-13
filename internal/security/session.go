package security

const (
	SessionCookieName = "fwlog_session"
	SessionMaxAge     = 86400
)

type SessionResponse struct {
	Authenticated bool `json:"authenticated"`
}
