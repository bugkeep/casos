package controllers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/casosorg/casos/proxy"
)

type dockerHubSearchResult struct {
	Count   int              `json:"count"`
	Results []dockerHubImage `json:"results"`
}

type dockerHubImage struct {
	Name        string `json:"repo_name"`
	Description string `json:"short_description"`
	IsOfficial  bool   `json:"is_official"`
	PullCount   int64  `json:"pull_count"`
	StarCount   int    `json:"star_count"`
}

type ImageSearchItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsOfficial  bool   `json:"isOfficial"`
	PullCount   int64  `json:"pullCount"`
	StarCount   int    `json:"starCount"`
	LogoURL     string `json:"logoUrl"`
}

func (c *ApiController) SearchDockerHubImages() {
	q := strings.TrimSpace(c.GetString("q"))
	if q == "" {
		c.ResponseError("query parameter 'q' is required")
		return
	}

	apiURL := fmt.Sprintf(
		"https://hub.docker.com/v2/search/repositories/?query=%s&page_size=20&page=1",
		url.QueryEscape(q),
	)

	resp, err := proxy.GetHttpClient(apiURL).Get(apiURL)
	if err != nil {
		c.ResponseError(fmt.Sprintf("failed to reach Docker Hub: %v", err))
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.ResponseError("failed to read Docker Hub response")
		return
	}

	var result dockerHubSearchResult
	if err := json.Unmarshal(body, &result); err != nil {
		c.ResponseError("failed to parse Docker Hub response")
		return
	}

	names := make([]string, len(result.Results))
	for i, r := range result.Results {
		names[i] = r.Name
	}
	nsLogos := FetchNamespaceLogos(uniqueNamespaces(names))

	items := make([]ImageSearchItem, len(result.Results))
	for i, r := range result.Results {
		items[i] = ImageSearchItem{
			Name:        r.Name,
			Description: r.Description,
			IsOfficial:  r.IsOfficial,
			PullCount:   r.PullCount,
			StarCount:   r.StarCount,
			LogoURL:     nsLogos[extractNamespace(r.Name)],
		}
	}

	c.ResponseOk(items)
}

type dockerHubTagsResult struct {
	Results []struct {
		Name string `json:"name"`
	} `json:"results"`
}

func (c *ApiController) GetDockerHubImageTags() {
	image := strings.TrimSpace(c.GetString("image"))
	if image == "" {
		c.ResponseError("image parameter is required")
		return
	}

	// Strip tag if caller passed full image:tag
	image = strings.SplitN(image, ":", 2)[0]

	parts := strings.SplitN(image, "/", 2)
	var namespace, name string
	if len(parts) == 1 {
		namespace = "library"
		name = parts[0]
	} else {
		namespace = parts[0]
		name = parts[1]
	}

	apiURL := fmt.Sprintf(
		"https://hub.docker.com/v2/repositories/%s/%s/tags/?page_size=30&ordering=last_updated",
		url.PathEscape(namespace), url.PathEscape(name),
	)

	resp, err := proxy.GetHttpClient(apiURL).Get(apiURL)
	if err != nil {
		c.ResponseError(fmt.Sprintf("failed to reach Docker Hub: %v", err))
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.ResponseError("failed to read Docker Hub response")
		return
	}

	var result dockerHubTagsResult
	if err := json.Unmarshal(body, &result); err != nil {
		c.ResponseError("failed to parse Docker Hub response")
		return
	}

	tags := make([]string, len(result.Results))
	for i, t := range result.Results {
		tags[i] = t.Name
	}

	c.ResponseOk(tags)
}
