package auth

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/dgrijalva/jwt-go"
)

func CreateToken(id uint) (string, error) {
	claims := jwt.MapClaims{}
	claims["authorized"] = true
	claims["id"] = id
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(os.Getenv("API_SECRET")))
}

// parseToken extracts and parses the JWT from the request. Shared by
// TokenValid and ExtractTokenID to avoid duplicating the signing-method
// check and secret lookup.
func parseToken(r *http.Request) (*jwt.Token, error) {
	tokenString := ExtractToken(r)
	return jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(os.Getenv("API_SECRET")), nil
	})
}

func TokenValid(r *http.Request) error {
	_, err := parseToken(r)
	return err
}

func ExtractToken(r *http.Request) string {
	keys := r.URL.Query()
	token := keys.Get("token")
	if token != "" {
		return token
	}
	bearerToken := r.Header.Get("Authorization")
	if len(strings.Split(bearerToken, " ")) == 2 {
		return strings.Split(bearerToken, " ")[1]
	}
	return ""
}

func ExtractTokenID(r *http.Request) (uint, error) {
	token, err := parseToken(r)
	if err != nil {
		return 0, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if ok && token.Valid {
		// Convert the `id` claim from string to uint
		uidStr := fmt.Sprintf("%v", claims["id"])
		uid, err := strconv.ParseUint(uidStr, 10, 32)
		if err != nil {
			return 0, err
		}
		return uint(uid), nil
	}
	return 0, fmt.Errorf("Invalid token")
}

