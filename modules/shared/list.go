package shared

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	jsonDbCache "github.com/anare/filejsondb"
	"github.com/kuetix/kue"
	"github.com/schollz/progressbar/v3"
)

type ListNames struct {
	dbm     *jsonDbCache.DB
	items   *jsonDbCache.Collection
	dbName  string
	colName string
}

func NewListNames(dbName string, colName string) *ListNames {
	return &ListNames{dbName: dbName, colName: colName}
}

func (l *ListNames) ListOfNames(pattern string, kueConfig KueConfig) (names []string, err error) {
	names = []string{}
	records, err := l.List(kueConfig)
	if len(records) > 0 {
		for _, record := range records {
			// path.Match's "*" does not cross "/", but workflow names are
			// slash-namespaced (cli/auth/login) — treat "*" as match-all.
			if pattern == "" || pattern == "*" {
				names = append(names, record)
				continue
			}
			if ok, err := path.Match(pattern, record); err == nil && ok {
				names = append(names, record)
			}
		}
	}

	return names, err
}

// GetDB initializes and returns the database connection
func (l *ListNames) GetDB(dbPaths ...string) (*jsonDbCache.Collection, error) {
	if l.items != nil {
		return l.items, nil
	}

	var dbPath string
	if len(dbPaths) > 0 {
		dbPath = dbPaths[0]
	} else {
		dbPath = filepath.Join(kue.HomeDir, kue.CacheDir, "cache")
	}

	dbFile := filepath.Join(dbPath, l.dbName)
	dbFullFile := filepath.Join(dbFile, l.colName)
	err := os.MkdirAll(dbFullFile, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}
	db, err := jsonDbCache.NewDB(dbFile)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	l.dbm = db
	l.items = l.dbm.NewCollection(l.colName)
	return l.items, nil
}

func (l *ListNames) List(kueConfig KueConfig) (records []string, err error) {
	records = []string{}
	var db *jsonDbCache.Collection
	// Get database connection
	db, err = l.GetDB()
	if err != nil {
		return records, err
	}

	var cursor string = ""
	var cursorDetails string = ""
	var count int = 0
	var limit = 100
	var entries map[string]interface{} = map[string]interface{}{}
	var body string = ""
	var urlPath string = ""
	page := 0
	pageMax := 100

	if db.Exists("list") {
		if err = db.Get("list", &entries); err == nil {
			if entries["timestamp"] != nil {
				if timestamp, ok := entries["timestamp"].(float64); ok {
					if time.Now().Unix()-int64(timestamp) < 300 {
						if entries["records"] != nil {
							for _, record := range entries["records"].([]interface{}) {
								records = append(records, record.(string))
							}
							fmt.Printf("Using cached workflow list (retrieved %d seconds ago)\n", time.Now().Unix()-int64(timestamp))
							return records, err
						}
					}
				}
			}
		}
	}
	fmt.Printf("Using server workflow list (cache expired or not found)\n")

	bar := progressbar.Default(int64(pageMax))
	for count >= 0 {
		urlPath = fmt.Sprintf("/workflow?limit=%d&cursor=%s", limit, cursor)
		body, _, err = PerformAuthenticatedRequest(kueConfig, http.MethodGet, urlPath, nil)
		if err != nil {
			return records, fmt.Errorf("workflow list failed: %w", err)
		}
		var obj map[string]interface{}
		if err = json.Unmarshal([]byte(body), &obj); err != nil {
			return records, fmt.Errorf("invalid workflow list JSON: %w", err)
		}

		data := obj["data"].(map[string]interface{})
		if data != nil {
			if c, ok := data["count"].(float64); ok {
				count = int(c)
			}

			if c, ok := data["cursor"].(string); ok {
				cursor = c
				var bin []byte
				bin, err = base64.StdEncoding.DecodeString(strings.TrimSpace(cursor))
				if err != nil {
					cursorDetails = ""
				} else {
					cursorDetails = string(bin)
					if cursorDetails == "" || cursorDetails == "|" {
						cursorDetails = ""
						cursor = ""
						count = -1
					}
				}
			} else {
				cursor = ""
				count = -1
			}
		}
		if cursor != "" {
			page++
			if page > pageMax {
				pageMax = bar.GetMax() + 1
				bar = progressbar.Default(int64(pageMax))
				_ = bar.Add(0)
			}
			_ = bar.Add(1)
		}

		for _, key := range []string{"workflows", "items", "data", "results"} {
			if v, ok := obj[key]; ok {
				if items, ok := v.([]interface{}); ok {
					records = append(records, NamesFromArray(items)...)
				}
			}
		}

		if d, ok := obj["data"].(map[string]interface{}); ok {
			for _, key := range []string{"workflows", "items", "results"} {
				if v, ok := d[key]; ok {
					if items, ok := v.([]interface{}); ok {
						records = append(records, NamesFromArray(items)...)
					}
				}
			}
		}
	}
	bar.ChangeMax(page)
	_ = bar.Finish()

	_ = db.Update("list", map[string]interface{}{
		"records":   records,
		"timestamp": time.Now().Unix(),
	})

	return records, nil
}

func NamesFromArray(items []interface{}) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		switch v := it.(type) {
		case string:
			if s := strings.TrimSpace(v); s != "" {
				out = append(out, s)
			}
		case map[string]interface{}:
			for _, key := range []string{"name", "workflow", "id"} {
				if s, ok := v[key].(string); ok && strings.TrimSpace(s) != "" {
					out = append(out, strings.TrimSpace(s))
					break
				}
			}
		}
	}
	return out
}
