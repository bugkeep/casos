package controllers

// Health answers as soon as the web server is listening: it touches no
// database, no session and no Kubernetes client, which is what makes it usable
// as a readiness probe while the rest of CasOS is still coming up.
//
// The "casos" payload identifies which service answered, so a caller polling
// the port can tell CasOS apart from whatever else may be bound to it.
func (c *ApiController) Health() {
	c.ResponseOk("casos")
}
