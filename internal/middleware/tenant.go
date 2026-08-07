package middleware

import (
	"control-panel/internal/application/services"
	"control-panel/internal/auth/jwtutil"

	"github.com/gin-gonic/gin"
)

// JWTAuth returns the default Casdoor-only auth middleware. Use
// JWTAuthWithCLI for routes that should also accept opaque cli_* tokens.
func JWTAuth() gin.HandlerFunc {
	return jwtutil.AuthMiddleware()
}

// JWTAuthWithCLI returns auth middleware that also accepts CLI tokens via the
// given CLITokenService.
func JWTAuthWithCLI(cliSvc *services.CLITokenService) gin.HandlerFunc {
	return jwtutil.AuthMiddlewareWithCLI(cliSvc)
}
