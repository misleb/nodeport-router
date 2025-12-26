package router

// This client supports the following routers:
// - Arris NVG443B

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

type Forward struct {
	PublicIP    string // The WAN IP of the router (not used but could be useful)
	DeviceName  string // The name of the cluster as known by the router
	ServiceName string // The name of the service that the forward is mapped to (namespace-name-port)
	Ports       string // The port on the WAN interface of the router that the forward is mapped to
	DevicePort  string // The port on the cluster that the forward is mapped to
	DeleteID    string // The ID of the forward in the router, used to delete it
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
		return err
	}

	c.setHeaders(req)

	if err = c.doRequest(req); err != nil {
		return err
	}

	c.lastLogin = time.Now()

	return nil
}

func (c *RouterClient) DeleteForward(forward Forward) error {
	forwards := []Forward{}
	nonce, err := c.GetForwards(&forwards)
	if err != nil {
		return fmt.Errorf("error getting forwards: %v", err)
	}
	deleteForwardURL := c.baseURL + "/cgi-bin/apphosting.ha"

	// Find the DeleteID in the forwards array
	deleteID := ""
	for _, f := range forwards {
		if f.ServiceName == forward.ServiceName {
			deleteID = f.DeleteID
			break
		}
	}
	if deleteID == "" {
		return fmt.Errorf("delete ID not found for service %s", forward.ServiceName)
	}

	data := url.Values{}
	data.Set("nonce", nonce)
	data.Set(deleteID, "Delete")

	req, err := http.NewRequest("POST", deleteForwardURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("error submitting delete forward: %v", err)
	}

	c.setHeaders(req)
	if err = c.doRequest(req); err != nil {
		return err
	}

	return nil
}

func (c *RouterClient) AddForward(forward Forward) error {
	forwards := []Forward{}

	// To add a forward we need to get the nonce to submit the form
	// We may never use the forwards array, but it's a convenient way to get the nonce
	nonce, err := c.GetForwards(&forwards)
	if err != nil {
		return fmt.Errorf("getting forwards: %v", err)
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
		return fmt.Errorf("submitting add forward: %v", err)
	}

	c.setHeaders(req)

	if err = c.doRequest(req); err != nil {
		return err
	}

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
			DeleteID:    cellTexts[4],
		})
	}

	nonce := extractNonce(string(bodyBytes))
	if nonce == "" {
		return "", fmt.Errorf("could not find nonce in apphosting page")
	}

	return nonce, nil
}

func (c *RouterClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", c.baseURL)
	req.Header.Set("Referer", req.URL.String())
}

func (c *RouterClient) doRequest(req *http.Request) error {
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %v", err)
	}
	// This is always a <meta http-equiv=Refresh content=0;url=...> response so we need
	// to explicitly do a GET followup to get the actual response body
	resp.Body.Close()
	resp, err = c.client.Get(req.URL.String())
	if err != nil {
		return fmt.Errorf("error making GET request: %v", err)
	}
	defer resp.Body.Close()

	// POST should always redirect to the same page so we need to check the body for error/success messages
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %v", err)
	}

	// Parse the HTML to check for error messages
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("error parsing HTML: %v", err)
	}

	// Check for error icon
	icon1 := findElementByID(doc, "error-message-icon")
	icon2 := findElementByAttr(doc, "src", "/images/icon_error.png")
	if icon1 != nil || icon2 != nil {
		errorDiv := findElementByID(doc, "error-message-text")
		if errorDiv != nil {
			errorText := strings.TrimSpace(getTextContent(errorDiv))
			if errorText != "" {
				return fmt.Errorf("%s", errorText)
			}
		}
	}

	return nil
}
