package router

// This client supports the following routers:
// - Arris NVG443B

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

type Forward struct {
	PublicIP    string
	DeviceName  string
	ServiceName string
	Ports       string
	DevicePort  string
}

type RouterClient struct {
	client     *http.Client
	baseURL    string
	username   string
	password   string
	sessionMux sync.Mutex
	lastLogin  time.Time
}

func NewRouterClient(baseURL, username, password string) *RouterClient {
	jar, _ := cookiejar.New(nil)

	return &RouterClient{
		client: &http.Client{
			Jar: jar,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		baseURL:  baseURL,
		username: username,
		password: password,
	}
}

func (c *RouterClient) Login() error {
	loginPageURL := c.baseURL + "/cgi-bin/login.ha"

	resp, err := c.client.Get(loginPageURL)
	if err != nil {
		fmt.Printf("Error fetching login page: %v\n", err)
		return err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return fmt.Errorf("error reading login page: %v", err)
	}

	nonce := extractNonce(string(body))
	if nonce == "" {
		return fmt.Errorf("could not find nonce in login page: %v", err)
	}

	// Step 2: Submit login with nonce
	data := url.Values{}
	data.Set("username", c.username)
	data.Set("password", c.password)
	data.Set("nonce", nonce)

	req, err := http.NewRequest("POST", loginPageURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("error submitting login: %v", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", c.baseURL)
	req.Header.Set("Referer", loginPageURL)

	resp, err = c.client.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %v", err)
	}
	resp.Body.Close()

	c.lastLogin = time.Now()

	return nil
}

func (c *RouterClient) AddForward(forward Forward) error {
	forwards := []Forward{}

	// To add a forward we need to get the nonce to submit the form
	// We may never use the forwards array, but it's a convenient way to get the nonce
	nonce, err := c.GetForwards(&forwards)
	if err != nil {
		return fmt.Errorf("error getting forwards: %v", err)
	}
	addForwardURL := c.baseURL + "/cgi-bin/apphosting.ha"
	data := url.Values{}
	data.Set("nonce", nonce)
	data.Set("device_select", forward.DeviceName)
	data.Set("device_manual", forward.DeviceName)
	data.Set("serviceName", forward.ServiceName)
	data.Set("service", "custom")
	data.Set("protocol", "both")
	data.Set("extMinPort", forward.Ports)
	data.Set("extMaxPort", "")
	data.Set("intStartPort", forward.DevicePort)
	data.Set("publicip", "")
	data.Set("Add", "Add")

	req, err := http.NewRequest("POST", addForwardURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("error submitting add forward: %v", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", c.baseURL)
	req.Header.Set("Referer", addForwardURL)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	return nil
}

func (r *RouterClient) EnsureLoggedIn() error {
	r.sessionMux.Lock()
	defer r.sessionMux.Unlock()

	// Re-login if session is older than X minutes
	if time.Since(r.lastLogin) > 5*time.Minute {
		return r.Login()
	}
	return nil
}

func (c *RouterClient) GetForwards(forwards *[]Forward) (string, error) {
	apphostingURL := c.baseURL + "/cgi-bin/apphosting.ha"
	resp, err := c.client.Get(apphostingURL)
	if err != nil {
		return "", fmt.Errorf("error fetching apphosting page: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %v", err)
	}

	// Parse the HTML
	doc, err := html.Parse(strings.NewReader(string(bodyBytes)))
	if err != nil {
		return "", fmt.Errorf("error parsing HTML: %v", err)
	}

	// Find and print the table with class="grid table100"
	table := findTableByClass(doc, "grid table100")
	if table == nil {
		return "", fmt.Errorf("could not find table with class 'grid table100'")
	}

	rows := findElements(table, "tr")
	for i, row := range rows {
		if i == 0 {
			// Skip the header row
			continue
		}
		cells := findElements(row, "td")

		var cellTexts []string
		for _, cell := range cells {
			text := strings.TrimSpace(getTextContent(cell))
			cellTexts = append(cellTexts, text)
		}

		*forwards = append(*forwards, Forward{
			PublicIP:    cellTexts[1],
			DeviceName:  cellTexts[0],
			ServiceName: cellTexts[2],
			Ports:       cellTexts[3],
		})
	}

	nonce := extractNonce(string(bodyBytes))
	if nonce == "" {
		return "", fmt.Errorf("could not find nonce in apphosting page")
	}

	return nonce, nil
}

// extractNonce finds the nonce value from a hidden input field
func extractNonce(htmlStr string) string {
	re := regexp.MustCompile(`name="nonce"[^>]*value="([^"]+)"`)
	if matches := re.FindStringSubmatch(htmlStr); len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// findTableByClass finds a table element with the specified class
func findTableByClass(n *html.Node, class string) *html.Node {
	if n.Type == html.ElementNode && n.Data == "table" {
		for _, attr := range n.Attr {
			if attr.Key == "class" && attr.Val == class {
				return n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if result := findTableByClass(c, class); result != nil {
			return result
		}
	}
	return nil
}

// findElements finds all elements with the given tag name within a node
func findElements(n *html.Node, tag string) []*html.Node {
	var results []*html.Node
	var find func(*html.Node)
	find = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == tag {
			results = append(results, node)
			return // Don't recurse into found elements
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			find(c)
		}
	}
	find(n)
	return results
}

// getTextContent extracts all text content from a node and its children
func getTextContent(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var text strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		text.WriteString(getTextContent(c))
	}
	return text.String()
}
