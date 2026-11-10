// Package jwtutil JWT 签发工具，等价于 TS jsonwebtoken 的 HS256 签发逻辑
package jwtutil

import (
	"time"

	"github.com/golang-jwt/jwt/v5"

	"itops-backend/internal/config"
)

// SignAccessToken 签发访问 token（claims: id/username/role/email/type=access，与 TS 一致）
func SignAccessToken(id, username, role string, email *string) (string, error) {
	expSec, err := config.ParseDuration(config.Env.JWTExpiresIn)
	if err != nil || expSec <= 0 {
		expSec = 24 * 3600
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"id":       id,
		"username": username,
		"role":     role,
		"email":    email,
		"type":     "access",
		"iat":      now.Unix(),
		"exp":      now.Add(time.Duration(expSec) * time.Second).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(config.Env.JWTSecret))
}

// SignRefreshToken 签发刷新 token（claims: id/type，有效期 7 天，与 TS 一致）
func SignRefreshToken(id string) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"id":   id,
		"type": "refresh",
		"iat":  now.Unix(),
		"exp":  now.Add(7 * 24 * time.Hour).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(config.Env.JWTSecret))
}
