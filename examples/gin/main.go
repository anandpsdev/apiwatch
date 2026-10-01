package main

import (
	"log"
	"net/http"

	"github.com/anandpsdev/apiwatch"
	"github.com/anandpsdev/apiwatch/config"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Default()

	watch, err := apiwatch.New(cfg)
	if err != nil {
		log.Fatalf("failed to initialize apiwatch: %v", err)
	}
	defer watch.Close()

	router := gin.Default()

	router.Use(watch.GinMiddleware())

	// Mount the APIWatch dashboard and API endpoints at /apiwatch
	watch.MountGin(router, "/apiwatch")

	// Sample application routes
	router.GET("/api/users", func(c *gin.Context) {
		c.JSON(http.StatusOK, []gin.H{
			{"id": 1, "name": "Alice"},
			{"id": 2, "name": "Bob"},
		})
	})

	router.POST("/api/users", func(c *gin.Context) {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		// The password will automatically be redacted in APIWatch
		c.JSON(http.StatusCreated, gin.H{"status": "created", "username": req.Username})
	})

	log.Println("APIWatch demo listening on http://localhost:8080")
	log.Println("Open dashboard at http://localhost:8080/apiwatch")
	if err := router.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
