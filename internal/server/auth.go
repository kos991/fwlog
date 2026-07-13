package server

import "fwlog/internal/security"

type SessionResponse = security.SessionResponse

func HashPassword(password string) (string, error) {
	return security.HashPassword(password)
}

func VerifyPassword(encoded, password string) bool {
	return security.VerifyPassword(encoded, password)
}

func looksLikePasswordHash(encoded string) bool {
	return security.LooksLikePasswordHash(encoded, minPasswordHashIterations)
}
