package domain

type Package struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      string `json:"author"`
	Email       string `json:"email"`
	License     string `json:"license"`
	Repository  string `json:"repository"`
}
