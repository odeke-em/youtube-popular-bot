package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/odeke-em/youtube"

	"github.com/ChimeraCoder/anaconda"
)

var (
	twitterAPI    *anaconda.TwitterApi
	youtubeClient *youtube.Client
)

var (
	twitterConsumerKey    = envValueAtInit("YOUTUBE_TWITTER_BOT_CONSUMER_KEY")
	twitterConsumerSecret = envValueAtInit("YOUTUBE_TWITTER_BOT_CONSUMER_SECRET")
	twitterAccessToken    = envValueAtInit("YOUTUBE_TWITTER_BOT_ACCESS_TOKEN")
	twitterAccessSecret   = envValueAtInit("YOUTUBE_TWITTER_BOT_ACCESS_SECRET")
)

var initErrMsgList = []string{}

func envValueAtInit(key string) string {
	value := os.Getenv(key)
	if value == "" {
		initErrMsgList = append(initErrMsgList, fmt.Sprintf("%q is not defined", key))
	}
	return value
}

func exitOnError(err error) {
	if err != nil {
		log.Fatalf("%v\n", err)
	}
}

func init() {
	if len(initErrMsgList) > 0 {
		msg := fmt.Sprintf("Errors Encountered:\n%s", strings.Join(initErrMsgList, "\n"))
		exitOnError(fmt.Errorf("%s", msg))
	}

	var err error
	youtubeClient, err = youtube.New()
	if err != nil {
		log.Fatal(err)
	}

	anaconda.SetConsumerKey(twitterConsumerKey)
	anaconda.SetConsumerSecret(twitterConsumerSecret)
	twitterAPI = anaconda.NewTwitterApi(twitterAccessToken, twitterAccessSecret)
}

func periodicTweets(period time.Duration) chan error {
	tick := time.Tick(period)
	errsChan := make(chan error)
	go func() {
		defer close(errsChan)

		for {

			since := time.Now().Add(-1 * period)
			param := &youtube.SearchParam{
				MaxPage: 2,

				MaxResultsPerPage: 10,
			}

			videoPages, err := youtubeClient.MostPopular(param)
			if err != nil {
				errsChan <- err
				<-tick
				break
			}

			tweetList := []*tweet{}
			for videoPage := range videoPages {
				if videoPage.Err != nil {
					errsChan <- videoPage.Err
					continue
				}

				for _, video := range videoPage.Items {
					snippet := video.Snippet
					stats := video.Statistics

					tw := &tweet{
						ViewCount:   stats.ViewCount,
						Title:       snippet.Title,
						YouTubeId:   video.Id,
						Description: snippet.Description,
					}
					tweetList = append(tweetList, tw)
				}
			}

			// Let's tweet them in reverse chronological order
			// and since the first will be the last to be tweeted,
			// the intro too is the last to be tweeted

			throttle := time.Tick(15 * time.Second)
			for rank := len(tweetList); rank > 0; rank-- {
				tw := tweetList[rank-1]
				tw.Rank = uint64(rank)
				tweetText, err := composeTweet(tw)
				if err != nil {
					errsChan <- err
				}

				result, err := twitterAPI.PostTweet(tweetText, nil)
				if err != nil {
					errsChan <- err
				}
				log.Printf("result: %v err: %s\n", result, err)
				<-throttle
			}

			introTweet := fmt.Sprintf("Most Popular/Trending %d YouTube videos for the last %s since %s", len(tweetList), period, since)

			if _, err := twitterAPI.PostTweet(introTweet, nil); err != nil {
				errsChan <- err
			}

			<-tick
		}
	}()

	return errsChan
}

const tweetTmplStr = `Rank #{{.Rank}} Views: {{.ViewCount}} Title: {{.Title}} {{youtubeURL .YouTubeId}}`

var tmplFuncs = template.FuncMap{
	"youtubeURL": func(id string) string { return fmt.Sprintf("https://youtu.be/%s", id) },
}
var tweetTemplate = template.Must(template.New("tweet").Funcs(tmplFuncs).Parse(tweetTmplStr))

func composeTweet(tw *tweet) (string, error) {
	buf := new(bytes.Buffer)
	if err := tweetTemplate.Execute(buf, tw); err != nil {
		return "", err
	}
	return string(buf.Bytes()), nil
}

type tweet struct {
	Rank        uint64
	ViewCount   uint64
	Title       string
	URL         string
	YouTubeId   string
	Description string
}

func main() {
	errsChan := periodicTweets(6 * time.Hour)
	for err := range errsChan {
		if err != nil {
			log.Printf("%v\n", err)
		}
	}
}
