package router

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// extractNonce finds the nonce value from a hidden input field
func extractNonce(htmlStr string) string {
	re := regexp.MustCompile(`name="nonce"[^>]*value="([^"]+)"`)
	if matches := re.FindStringSubmatch(htmlStr); len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func findElementByAttr(n *html.Node, attribute, value string) *html.Node {
	if n.Type == html.ElementNode {
		for _, attr := range n.Attr {
			if attr.Key == attribute && attr.Val == value {
				return n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if result := findElementByAttr(c, attribute, value); result != nil {
			return result
		}
	}
	return nil
}

// findElementByID finds an element with the specified ID
func findElementByID(n *html.Node, id string) *html.Node {
	if n.Type == html.ElementNode {
		for _, attr := range n.Attr {
			if attr.Key == "id" && attr.Val == id {
				return n
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if result := findElementByID(c, id); result != nil {
			return result
		}
	}
	return nil
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
		// Extract the name attribute from the input element. This is the Delete ID for the forward.
		if c.Type == html.ElementNode && c.Data == "input" {
			for _, attr := range c.Attr {
				if attr.Key == "name" {
					text.WriteString(attr.Val)
				}
			}
		} else {
			text.WriteString(getTextContent(c))
		}
	}
	return text.String()
}
