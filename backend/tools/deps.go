//go:build tools

// Package tools pins dependencies that modules use, so `go mod tidy` keeps them and parallel
// work never needs to edit go.mod. Add a dependency here first, then use it.
package tools

import (
	_ "github.com/DATA-DOG/go-sqlmock"
	_ "github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
	_ "github.com/coder/websocket"
	_ "github.com/pquerna/otp/totp"
	_ "golang.org/x/crypto/bcrypt"
)
