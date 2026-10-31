// Fixture: three declared Go modules, only gin is imported. viper and
// golang.org/x/crypto are therefore phantom dependencies.
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	_ = r.Run(":8081")
}
