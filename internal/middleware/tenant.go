package middleware

import (
	"control-panel/internal/application/services"
	"control-panel/internal/auth"
	"control-panel/internal/auth/jwtutil"

	"github.com/gin-gonic/gin"
)

// JWTAuth returns auth middleware that accepts provider-validated access tokens
// only. Use JWTAuthWithCLI for routes that should also accept opaque cli_*
// tokens.
func JWTAuth(p auth.Provider) gin.HandlerFunc {
	return jwtutil.AuthMiddlewareWithCLI(nil, p)
}

// JWTAuthWithCLI returns auth middleware that also accepts CLI tokens via the
// given CLITokenService.
func JWTAuthWithCLI(cliSvc *services.CLITokenService, p auth.Provider) gin.HandlerFunc {
	return jwtutil.AuthMiddlewareWithCLI(cliSvc, p)
}
