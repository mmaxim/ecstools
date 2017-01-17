package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"encoding/json"

	"bytes"

	"strings"

	"github.com/mmaxim/ecstools/libecs"
)

func main() {
	rc := mainInner()
	os.Exit(rc)
}

type Options struct {
	ClusterName     string
	Region          string
	ShortArns       bool
	KeybaseLocation string
}

func runServiceOutput(opts Options, out io.Writer, ecs *libecs.ECS) error {
	ecs, err := libecs.New(libecs.ECSConfig{
		Cluster: opts.ClusterName,
		Region:  opts.Region,
	})
	if err != nil {
		fmt.Printf("failed to create ECS API object: %s\n", err.Error())
		return err
	}

	services, err := ecs.ListServices()
	if err != nil {
		fmt.Printf("failed to list services: %s\n", err.Error())
	}

	output := libecs.NewBasicServiceOutputer(opts.ShortArns)

	if err := output.Display(services, out); err != nil {
		fmt.Printf("failed to display: %s\n", err.Error())
		return err
	}
	return nil
}

type Conversation struct {
	Id     string `json:"id"`
	Unread bool   `json:"unread"`
}

type Result struct {
	Convs []Conversation `json:"conversations"`
}

type Inbox struct {
	Result Result `json:"result"`
}

func getUnreadConvos(inboxRaw string) ([]string, error) {
	var inbox Inbox
	var res []string
	if err := json.Unmarshal([]byte(inboxRaw[:]), &inbox); err != nil {
		return nil, err
	}
	for _, conv := range inbox.Result.Convs {
		if conv.Unread {
			res = append(res, conv.Id)
			fmt.Printf("id: %s unread: %v\n", conv.Id, conv.Unread)
		}
	}

	return res, nil
}

type Text struct {
	Body string `json:"body"`
}

type Content struct {
	Type string `json:"type"`
	Text Text   `json:"text"`
}

type Message struct {
	Content Content `json:"content"`
}

type MessageHolder struct {
	Msg Message `json:"msg"`
}

type ThreadResult struct {
	Messages []MessageHolder `json:"messages"`
}

type Thread struct {
	Result ThreadResult `json:"result"`
}

func shouldSendToConv(convID string, input io.Writer, output io.Reader) (bool, error) {
	read := fmt.Sprintf("{\"method\": \"read\", \"params\": {\"options\": {\"conversation_id\": \"%s\", \"unread_only\": true}}}", convID)
	io.WriteString(input, read)
	o := bufio.NewScanner(output)
	o.Scan()

	var thread Thread
	if err := json.Unmarshal([]byte(o.Text()), &thread); err != nil {
		return false, fmt.Errorf("unable to decode thread: %s", err.Error())
	}

	for _, msg := range thread.Result.Messages {
		if msg.Msg.Content.Type == "text" && msg.Msg.Content.Text.Body == "ecslist" {
			return true, nil
		}
	}

	return false, nil
}

func getConvsToSend(convIDs []string, input io.Writer, output io.Reader) ([]string, error) {
	var res []string
	for _, convID := range convIDs {
		shouldSend, err := shouldSendToConv(convID, input, output)
		if err != nil {
			return nil, err
		}
		if shouldSend {
			res = append(res, convID)
			fmt.Printf("sending to: %s\n", convID)
		}
	}
	return res, nil
}

func sendToConvs(convIDs []string, outputRes string, input io.Writer, output io.Reader) error {
	outputRes = strings.Replace(outputRes, "\n", "\\n", -1)
	for _, convID := range convIDs {
		send := fmt.Sprintf("{\"method\": \"send\", \"params\": {\"options\": {\"conversation_id\": \"%s\", \"message\": {\"body\": \"```%s```\"}}}}", convID, outputRes)
		io.WriteString(input, send)
		o := bufio.NewScanner(output)
		o.Scan()
	}
	return nil
}

func chatOnce(opts Options, ecs *libecs.ECS, input io.Writer, output io.Reader) error {

	list := "{\"method\":\"list\"}"
	io.WriteString(input, list)

	o := bufio.NewScanner(output)
	o.Scan()

	convIDs, err := getUnreadConvos(o.Text())
	if err != nil {
		return err
	}

	convIDs, err = getConvsToSend(convIDs, input, output)
	if err != nil {
		return err
	}

	if len(convIDs) > 0 {
		var ecsInfo bytes.Buffer
		if err := runServiceOutput(opts, &ecsInfo, ecs); err != nil {
			return err
		}
		if err := sendToConvs(convIDs, ecsInfo.String(), input, output); err != nil {
			return err
		}
	}
	return nil
}

func chatLoop(opts Options, ecs *libecs.ECS) error {

	// Start up KB chat
	p := exec.Command(opts.KeybaseLocation, "chat", "api")
	input, err := p.StdinPipe()
	if err != nil {
		return err
	}
	output, err := p.StdoutPipe()
	if err != nil {
		return err
	}
	if err := p.Start(); err != nil {
		return err
	}

	chatOnce(opts, ecs, input, output)
	for {
		select {
		case <-time.After(2 * time.Second):
			if err := chatOnce(opts, ecs, input, output); err != nil {
				return err
			}
		}
	}
}

func mainInner() int {
	var opts Options

	flag.StringVar(&opts.KeybaseLocation, "keybase", "keybase", "keybase command")
	flag.StringVar(&opts.ClusterName, "cluster", "gregord", "cluster name")
	flag.StringVar(&opts.Region, "region", "us-east-1", "AWS region name")
	flag.BoolVar(&opts.ShortArns, "short-arns", true, "display only last part of ARN")
	flag.Parse()

	ecs, err := libecs.New(libecs.ECSConfig{
		Cluster: opts.ClusterName,
		Region:  opts.Region,
	})

	if err != nil {
		fmt.Printf("failed to create ECS API object: %s", err.Error())
		return 3
	}
	if err := chatLoop(opts, ecs); err != nil {
		fmt.Printf("error running chat loop: %s\n", err.Error())
	}

	return 0
}
