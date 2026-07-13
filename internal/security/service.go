package security

type Service interface {
	HashPassword(password string) (string, error)
	VerifyPassword(encoded, password string) bool
	LooksLikePasswordHash(encoded string, minIterations int) bool
}

type PasswordService struct{}

func (PasswordService) HashPassword(password string) (string, error) {
	return HashPassword(password)
}

func (PasswordService) VerifyPassword(encoded, password string) bool {
	return VerifyPassword(encoded, password)
}

func (PasswordService) LooksLikePasswordHash(encoded string, minIterations int) bool {
	return LooksLikePasswordHash(encoded, minIterations)
}
