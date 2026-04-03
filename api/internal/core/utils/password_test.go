package utils

import (
	"testing"
)

func TestValidatePasswordComplexity(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  error
	}{
		{
			name:     "valid password",
			password: "StrongPassword1!",
			wantErr:  nil,
		},
		{
			name:     "too short",
			password: "Short1!",
			wantErr:  ErrPasswordTooShort,
		},
		{
			name:     "no uppercase",
			password: "weakpassword1!",
			wantErr:  ErrPasswordNoUppercase,
		},
		{
			name:     "no lowercase",
			password: "WEAKPASSWORD1!",
			wantErr:  ErrPasswordNoLowercase,
		},
		{
			name:     "no number",
			password: "NoNumberPassword!",
			wantErr:  ErrPasswordNoNumber,
		},
		{
			name:     "no special char",
			password: "NoSpecialChar123",
			wantErr:  ErrPasswordNoSpecialChar,
		},
		{
			name:     "valid with other special char",
			password: "Another@Pass99",
			wantErr:  nil,
		},
		{
			name:     "valid with space as special char",
			password: "Password With Space 1",
			wantErr:  ErrPasswordNoSpecialChar, // unicode.IsPunct or IsSymbol usually don't include space
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePasswordComplexity(tt.password)
			if err != tt.wantErr {
				t.Errorf("ValidatePasswordComplexity(%q) error = %v, wantErr %v", tt.password, err, tt.wantErr)
			}
		})
	}
}
