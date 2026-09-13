package security

const (
	SessionCookieName = "fwlog_session"
	SessionMaxAge     = 86400
)

type SessionResponse struct {
	Authenticated bool   `json:"authenticated"`
	MFARequired   bool   `json:"mfa_required,omitempty"`
	Challenge     string `json:"challenge,omitempty"`
	AccessToken   string `json:"access_token,omitempty"`
	TokenType     string `json:"token_type,omitempty"`
}
