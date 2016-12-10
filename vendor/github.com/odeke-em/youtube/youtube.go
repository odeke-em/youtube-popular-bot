package youtube

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	googleapiTransport "google.golang.org/api/googleapi/transport"
	"google.golang.org/api/youtube/v3"
)

var (
	envAPIKeyKey   = "YOUTUBE_API_KEY"
	envResolvedKey = strings.TrimSpace(os.Getenv(envAPIKeyKey))
)

type Client struct {
	sync.RWMutex
	apiKey  string
	service *youtube.Service
}

var (
	errEmptyEnvAPIKey = fmt.Errorf("empty API Key from environment. Expecting env %q", envAPIKeyKey)
	errEmptyAPIKey    = fmt.Errorf("expecting a non-empty API key")
)

func clientWithKey(key string) (*Client, error) {
	httpClient := &http.Client{
		Transport: &googleapiTransport.APIKey{Key: key},
	}

	service, err := youtube.New(httpClient)
	if err != nil {
		return nil, err
	}

	client := new(Client)
	client.apiKey = key
	client.service = service

	return client, nil
}

// New returns a client with an API Key derived
// from the environment, set as YOUTUBE_API_KEY.
func New() (*Client, error) {
	apiKey := envResolvedKey
	if apiKey == "" {
		return nil, errEmptyEnvAPIKey
	}
	return clientWithKey(envResolvedKey)
}

// NewWithKey creates a client
// with the provided API Key.
func NewWithKey(apiKey string) (*Client, error) {
	if apiKey == "" {
		return nil, errEmptyAPIKey
	}
	return clientWithKey(envResolvedKey)
}

type SearchParam struct {
	PageToken string `json:"page_token"`

	// Query is the content to search for.
	Query string `json:"query"`

	// MaxPage is the maximum number of
	// pages of items that you want returned
	MaxPage uint64 `json:"max_page"`

	// MaxResultsPerPage is the maximum number of
	// items to be returned per pagination/page fetch.
	MaxResultsPerPage uint64 `json:"max_results_per_page"`

	// MaxRequestedItems is the threshold for the number
	// of items that you'd like to stop the search after.
	MaxRequestedItems uint64 `json:"max_requested_items"`

	// RelatedToVideoId is the id for whose
	// related videos you'd like returned
	RelatedToVideoId string `json:"related_to_video_id"`
}

type SearchPage struct {
	Index uint64

	Err   error
	Items []*youtube.SearchResult
}

type ResultsPage struct {
	Index uint64
	Err   error
	Items []*youtube.Video
}

var videoListFields = "id,snippet,statistics"

func (c *Client) ById(ids ...string) (chan *ResultsPage, error) {
	idsCSV := strings.Join(ids, ",")
	req := c.service.Videos.List(videoListFields).Id(idsCSV)
	return c.doVideos(req, nil)
}

// MostPopular returns the currently most popular videos.
// Specifying MaxPage, MaxResultsPerPage help
// control how many items should be retrieved.
func (c *Client) MostPopular(param *SearchParam) (chan *ResultsPage, error) {
	req := c.service.Videos.List(videoListFields).Chart("mostPopular")
	return c.doVideos(req, param)
}

func (c *Client) doVideos(req *youtube.VideosListCall, param *SearchParam) (chan *ResultsPage, error) {
	pagesChan := make(chan *ResultsPage)

	if param == nil {
		param = new(SearchParam)
	}

	go func() {
		defer close(pagesChan)
		ticker := time.NewTicker(1e8)
		defer ticker.Stop()

		maxPageIndex := param.MaxPage
		maxResultsPerPage := param.MaxResultsPerPage
		maxRequestedItems := param.MaxRequestedItems

		pageIndex := uint64(0)
		itemsCount := uint64(0)
		pageToken := param.PageToken

		for {
			if maxRequestedItems > 0 && itemsCount >= maxRequestedItems {
				break
			}

			if maxPageIndex > 0 && pageIndex >= maxPageIndex {
				break
			}

			// If there are still more pages, let's keep searching
			if pageToken != "" {
				req = req.PageToken(pageToken)
			}

			if maxResultsPerPage > 0 {
				req = req.MaxResults(int64(maxResultsPerPage))
			}

			res, err := req.Do()
			if err != nil {
				pagesChan <- &ResultsPage{Err: err, Index: pageIndex}
				return
			}

			pageToken = res.NextPageToken

			// Increment our stoppers and that pageIndex
			itemsCount += uint64(len(res.Items))
			pageIndex += 1

			page := &ResultsPage{
				Index: pageIndex,
				Items: res.Items,
			}

			pagesChan <- page

			if pageToken == "" {
				break
			}

			<-ticker.C
		}
	}()

	return pagesChan, nil
}

func (c *Client) Search(param *SearchParam) (chan *SearchPage, error) {
	pagesChan := make(chan *SearchPage)

	go func() {
		defer close(pagesChan)
		ticker := time.NewTicker(1e8)
		defer ticker.Stop()

		query := param.Query
		maxPageIndex := param.MaxPage
		maxResultsPerPage := param.MaxResultsPerPage
		maxRequestedItems := param.MaxRequestedItems

		req := c.service.Search.List("id,snippet").Q(query)
		if maxResultsPerPage > 0 {
			req = req.MaxResults(int64(maxResultsPerPage))
		}

		if param.RelatedToVideoId != "" {
			// When RelatedToVideo is used, we must set Type to "video"
			req = req.RelatedToVideoId(param.RelatedToVideoId).Type("video")
		}

		pageIndex := uint64(0)
		itemsCount := uint64(0)
		pageToken := param.PageToken

		for {
			if maxRequestedItems > 0 && itemsCount >= maxRequestedItems {
				break
			}

			if maxPageIndex > 0 && pageIndex >= maxPageIndex {
				break
			}

			// If there are still more pages, let's keep searching
			if pageToken != "" {
				req = req.PageToken(pageToken)
			}

			res, err := req.Do()
			if err != nil {
				pagesChan <- &SearchPage{Err: err, Index: pageIndex}
				return
			}

			pageToken = res.NextPageToken

			// Increment our stoppers and that pageIndex
			itemsCount += uint64(len(res.Items))
			pageIndex += 1

			page := &SearchPage{
				Index: pageIndex,
				Items: res.Items,
			}

			pagesChan <- page

			if pageToken == "" {
				break
			}

			<-ticker.C
		}
	}()

	return pagesChan, nil
}
