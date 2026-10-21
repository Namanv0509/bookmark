package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/lib/pq"
)

type Link struct {
	ID        int64     `json:"id"`
	URL       string    `json:"url"`
	Title     string    `json:"title"`
	Notes     string    `json:"notes"`
	Tags      []string  `json:"tags"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type LinkInput struct {
	URL   string   `json:"url"`
	Title string   `json:"title"`
	Notes string   `json:"notes"`
	Tags  []string `json:"tags"`
}

type API struct {
	db *sql.DB
}

func main() {
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USER", "postgres")
	password := getEnv("DB_PASSWORD", "password")
	name := getEnv("DB_NAME", "linkvault")
	sslmode := getEnv("DB_SSLMODE", "disable")

	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, name, sslmode,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal("Could not configure database:", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	// Wait briefly for PostgreSQL to become reachable.
	var connected bool
	for attempt := 1; attempt <= 15; attempt++ {
		if err := db.Ping(); err == nil {
			connected = true
			break
		}

		log.Printf("Waiting for PostgreSQL (%d/15)...", attempt)
		time.Sleep(2 * time.Second)
	}

	if !connected {
		log.Fatal("Could not connect to PostgreSQL")
	}

	log.Println("Connected to PostgreSQL")

	api := &API{db: db}
	e := echo.New()

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.File("/", "index.html")

	e.GET("/health", func(c echo.Context) error {
		if err := db.PingContext(c.Request().Context()); err != nil {
			return c.JSON(503, map[string]string{
				"status": "database unavailable",
			})
		}
		return c.JSON(200, map[string]string{"status": "ok"})
	})

	e.POST("/links", api.createLink)
	e.GET("/links", api.listLinks)
	e.GET("/links/:id", api.getLink)
	e.PUT("/links/:id", api.updateLink)
	e.DELETE("/links/:id", api.deleteLink)

	portHTTP := getEnv("PORT", "8080")
	log.Printf("LinkVault API listening on :%s", portHTTP)
	log.Fatal(e.Start(":" + portHTTP))
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func validateInput(input LinkInput) error {
	if strings.TrimSpace(input.Title) == "" {
		return errors.New("title is required")
	}

	if strings.TrimSpace(input.URL) == "" {
		return errors.New("url is required")
	}

	parsed, err := url.ParseRequestURI(input.URL)
	if err != nil || parsed.Host == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("url must be a valid HTTP or HTTPS URL")
	}

	return nil
}

func (api *API) createLink(c echo.Context) error {
	var input LinkInput
	if err := c.Bind(&input); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid JSON"})
	}

	if err := validateInput(input); err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}

	if input.Tags == nil {
		input.Tags = []string{}
	}

	var link Link
	err := api.db.QueryRowContext(
		c.Request().Context(),
		`INSERT INTO links (url, title, notes, tags)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, url, title, notes, tags, created_at, updated_at`,
		input.URL, input.Title, input.Notes, pq.Array(input.Tags),
	).Scan(
		&link.ID, &link.URL, &link.Title, &link.Notes,
		pq.Array(&link.Tags), &link.CreatedAt, &link.UpdatedAt,
	)
	if err != nil {
		log.Printf("Create link failed: %v", err)
		return c.JSON(500, map[string]string{"error": "could not save link"})
	}

	return c.JSON(201, link)
}

func (api *API) listLinks(c echo.Context) error {
	rows, err := api.db.QueryContext(
		c.Request().Context(),
		`SELECT id, url, title, notes, tags, created_at, updated_at
		 FROM links ORDER BY created_at DESC`,
	)
	if err != nil {
		return c.JSON(500, map[string]string{"error": "could not retrieve links"})
	}
	defer rows.Close()

	links := []Link{}
	for rows.Next() {
		var link Link
		if err := rows.Scan(
			&link.ID, &link.URL, &link.Title, &link.Notes,
			pq.Array(&link.Tags), &link.CreatedAt, &link.UpdatedAt,
		); err != nil {
			return c.JSON(500, map[string]string{"error": "could not read links"})
		}
		if link.Tags == nil {
			link.Tags = []string{}
		}
		links = append(links, link)
	}

	if err := rows.Err(); err != nil {
		return c.JSON(500, map[string]string{"error": "could not read links"})
	}

	return c.JSON(200, links)
}

func (api *API) getLink(c echo.Context) error {
	id := c.Param("id")

	var link Link
	err := api.db.QueryRowContext(
		c.Request().Context(),
		`SELECT id, url, title, notes, tags, created_at, updated_at
		 FROM links WHERE id = $1`,
		id,
	).Scan(
		&link.ID, &link.URL, &link.Title, &link.Notes,
		pq.Array(&link.Tags), &link.CreatedAt, &link.UpdatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return c.JSON(404, map[string]string{"error": "link not found"})
	}
	if err != nil {
		return c.JSON(500, map[string]string{"error": "could not retrieve link"})
	}

	if link.Tags == nil {
		link.Tags = []string{}
	}
	return c.JSON(200, link)
}

func (api *API) updateLink(c echo.Context) error {
	var input LinkInput
	if err := c.Bind(&input); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid JSON"})
	}

	if err := validateInput(input); err != nil {
		return c.JSON(400, map[string]string{"error": err.Error()})
	}

	if input.Tags == nil {
		input.Tags = []string{}
	}

	var link Link
	err := api.db.QueryRowContext(
		c.Request().Context(),
		`UPDATE links
		 SET url = $1, title = $2, notes = $3, tags = $4,
		     updated_at = NOW()
		 WHERE id = $5
		 RETURNING id, url, title, notes, tags, created_at, updated_at`,
		input.URL, input.Title, input.Notes, pq.Array(input.Tags), c.Param("id"),
	).Scan(
		&link.ID, &link.URL, &link.Title, &link.Notes,
		pq.Array(&link.Tags), &link.CreatedAt, &link.UpdatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return c.JSON(404, map[string]string{"error": "link not found"})
	}
	if err != nil {
		log.Printf("Update link failed: %v", err)
		return c.JSON(500, map[string]string{"error": "could not update link"})
	}

	return c.JSON(200, link)
}

func (api *API) deleteLink(c echo.Context) error {
	result, err := api.db.ExecContext(
		c.Request().Context(),
		`DELETE FROM links WHERE id = $1`,
		c.Param("id"),
	)
	if err != nil {
		return c.JSON(500, map[string]string{"error": "could not delete link"})
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return c.JSON(500, map[string]string{"error": "could not confirm deletion"})
	}
	if affected == 0 {
		return c.JSON(404, map[string]string{"error": "link not found"})
	}

	return c.JSON(200, map[string]string{"message": "link deleted"})
}
