package utils

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"
)

func GetCertExpiration(path string) (time.Time, error) {
	certPEM, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, err
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return time.Time{}, fmt.Errorf("certificate PEM block not found")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, err
	}
	return cert.NotAfter, nil
}
