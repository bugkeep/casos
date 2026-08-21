package controllers

import "strconv"

type Response struct {
	Status string      `json:"status"`
	Msg    string      `json:"msg"`
	Data   interface{} `json:"data"`
	Data2  interface{} `json:"data2"`
}

func (c *ApiController) ResponseOk(data ...interface{}) {
	resp := Response{Status: "ok"}
	switch len(data) {
	case 2:
		resp.Data2 = data[1]
		fallthrough
	case 1:
		resp.Data = data[0]
	}
	c.Data["json"] = resp
	c.ServeJSON()
}

func (c *ApiController) ResponseError(error string, data ...interface{}) {
	resp := Response{Status: "error", Msg: error}
	switch len(data) {
	case 2:
		resp.Data2 = data[1]
		fallthrough
	case 1:
		resp.Data = data[0]
	}
	c.Data["json"] = resp
	c.ServeJSON()
}

func (c *ApiController) RequireSignedIn() bool {
	if c.GetSessionUser() == nil {
		c.ResponseError("please sign in first")
		return true
	}

	return false
}

func (c *ApiController) RequireAdmin() bool {
	return c.RequireSignedIn()
}

// Pagination defaults. limit is clamped to maxLimit so a single curl cannot
// ask the apiserver to materialize an unbounded slice in memory.
const (
	defaultPage  = 1
	defaultLimit = 20
	maxLimit     = 500
)

// parsePagination reads page/limit from the query string. page is 1-based and
// defaults to 1; limit defaults to defaultLimit and is clamped to [1, maxLimit].
// paged reports whether the caller asked for paging at all — list endpoints
// keep returning the legacy bare-array shape when it is false, so existing
// callers do not have to migrate in lockstep.
func parsePagination(c *ApiController) (page, limit int, paged bool) {
	pageStr := c.GetString("page")
	limitStr := c.GetString("limit")
	if pageStr == "" && limitStr == "" {
		return defaultPage, defaultLimit, false
	}
	paged = true
	page = defaultPage
	if pageStr != "" {
		if parsed, err := strconv.Atoi(pageStr); err == nil && parsed > 0 {
			page = parsed
		}
	}
	limit = defaultLimit
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	return page, limit, true
}

// paginateSlice returns the page-bounded subset of items together with the
// total. Caller is expected to have sorted the input already; this function
// only bounds the slice.
func paginateSlice[T any](items []T, page, limit int) ([]T, int) {
	total := len(items)
	start := (page - 1) * limit
	if start < 0 || start >= total {
		return []T{}, total
	}
	end := start + limit
	if end > total {
		end = total
	}
	return items[start:end], total
}
