package utils

import (
	"errors"
	"unicode"
)

var (
	ErrPasswordTooShort        = errors.New("password must be at least 12 characters")
	ErrPasswordNoUppercase     = errors.New("password must contain at least one uppercase letter")
	ErrPasswordNoLowercase     = errors.New("password must contain at least one lowercase letter")
	ErrPasswordNoNumber        = errors.New("password must contain at least one number")
	ErrPasswordNoSpecialChar   = errors.New("password must contain at least one special character")
)

// ValidatePasswordComplexity checks if a password meets the required complexity standards.
func ValidatePasswordComplexity(password string) error {
	if len(password) < 12 {
		return ErrPasswordTooShort
	}

	var (
		hasUpper   bool
		hasLower   bool
		hasNumber  bool
		hasSpecial bool
	)

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char):
			hasNumber = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return ErrPasswordNoUppercase
	}
	if !hasLower {
		return ErrPasswordNoLowercase
	}
	if !hasNumber {
		return ErrPasswordNoNumber
	}
	if !hasSpecial {
		return ErrPasswordNoSpecialChar
	}

	return nil
}
