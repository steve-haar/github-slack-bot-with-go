package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"sync"

	"github.com/google/go-github/v45/github"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

var REPO_OWNER string
var REPO_NAME string

func main() {
	REPO_OWNER = os.Getenv("REPO_OWNER")
	REPO_NAME = os.Getenv("REPO_NAME")
	OAUTH_TOKEN := os.Getenv("OAUTH_TOKEN")
	APP_TOKEN := os.Getenv("APP_TOKEN")

	githubApiClient := github.NewClient(nil)
	slackApiClient := slack.New(OAUTH_TOKEN, slack.OptionAppLevelToken(APP_TOKEN))
	slackSocketClient := socketmode.New(slackApiClient)

	go func() {
		for {
			event := <-slackSocketClient.Events

			if event.Type == socketmode.EventTypeEventsAPI {
				slackSocketClient.Ack(*event.Request)

				apiEvent, ok := event.Data.(slackevents.EventsAPIEvent)
				if ok && apiEvent.Type == slackevents.CallbackEvent {
					switch eventData := apiEvent.InnerEvent.Data.(type) {
					case *slackevents.MessageEvent:
						handleMessage(githubApiClient, slackApiClient, eventData)
					}
				}
			}
		}
	}()

	go func() {
		err := slackSocketClient.Run()
		if err != nil {
			panic(err)
		}
	}()

	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		fmt.Fprintf(writer, "Hello World!")
	})

	err := http.ListenAndServe(":"+os.Getenv("PORT"), nil)
	if err != nil {
		panic(err)
	}
}

func handleMessage(githubApiClient *github.Client, slackApiClient *slack.Client, eventData *slackevents.MessageEvent) {
	issueNumbers := getIssueNumbers(eventData.Text)
	issues := getIssues(githubApiClient, issueNumbers)
	wg := sync.WaitGroup{}

	for _, issue := range issues {
		wg.Add(1)
		go func(issue *github.Issue) {
			attachment := getAttachment(issue)
			slackApiClient.PostMessage(eventData.Channel, slack.MsgOptionAttachments(attachment))
			wg.Done()
		}(issue)
	}

	wg.Wait()
}

func getIssueNumbers(text string) []int {
	issueRegex, _ := regexp.Compile(`#(\d+)`)
	matches := issueRegex.FindAllStringSubmatch(text, -1)
	numbers := make([]int, 0, len(matches))

	for _, match := range matches {
		number, err := strconv.Atoi(match[1])
		if err == nil {
			numbers = append(numbers, number)
		}
	}

	return numbers
}

func getIssues(githubApiClient *github.Client, issueNumbers []int) []*github.Issue {
	issues := make([]*github.Issue, 0, len(issueNumbers))
	wg := sync.WaitGroup{}

	for _, issueNumber := range issueNumbers {
		wg.Add(1)
		go func(issueNumber int) {
			issue, _, err := githubApiClient.Issues.Get(context.Background(), REPO_OWNER, REPO_NAME, issueNumber)
			if err == nil {
				issues = append(issues, issue)
			}
			wg.Done()
		}(issueNumber)
	}

	wg.Wait()

	return issues
}

func getAttachment(issue *github.Issue) slack.Attachment {
	return slack.Attachment{
		Color:   "#76b900",
		Pretext: fmt.Sprintf("%s (#%d)", *issue.Title, *issue.Number),
		Fields: []slack.AttachmentField{
			{
				Title: "Created",
				Value: issue.CreatedAt.Format("January 2, 2006"),
			},
			{
				Title: "Status",
				Value: *issue.State,
			},
		},
	}
}
