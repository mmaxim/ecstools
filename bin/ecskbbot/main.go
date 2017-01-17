package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"bytes"

	"strings"

	"github.com/mmaxim/ecstools/bin/ecskbbot/kbchat"
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

type BotServer struct {
	opts Options
	ecs  *libecs.ECS

	kbc *kbchat.API
}

func NewBotServer(opts Options, ecs *libecs.ECS) *BotServer {
	return &BotServer{
		opts: opts,
		ecs:  ecs,
	}
}

func (s *BotServer) runServiceOutput(out io.Writer) error {
	ecs, err := libecs.New(libecs.ECSConfig{
		Cluster: s.opts.ClusterName,
		Region:  s.opts.Region,
	})
	if err != nil {
		fmt.Printf("failed to create ECS API object: %s\n", err.Error())
		return err
	}

	services, err := ecs.ListServices()
	if err != nil {
		fmt.Printf("failed to list services: %s\n", err.Error())
	}

	output := libecs.NewBasicServiceOutputer(s.opts.ShortArns)

	if err := output.Display(services, out); err != nil {
		fmt.Printf("failed to display: %s\n", err.Error())
		return err
	}
	return nil
}

func (s *BotServer) shouldSendToConv(convID string) (bool, error) {

	msgs, err := s.kbc.GetTextMessages(convID, true)
	if err != nil {
		return false, err
	}
	for _, msg := range msgs {
		if msg.Content.Type == "text" && msg.Content.Text.Body == "!ecslist" {
			return true, nil
		}
	}

	return false, nil
}

func (s *BotServer) getConvsToSend(convs []kbchat.Conversation) ([]kbchat.Conversation, error) {
	var res []kbchat.Conversation
	for _, conv := range convs {
		shouldSend, err := s.shouldSendToConv(conv.Id)
		if err != nil {
			return nil, err
		}
		if shouldSend {
			res = append(res, conv)
			fmt.Printf("sending to: id: %s name: %s\n", conv.Id, conv.Channel.Name)
		}
	}
	return res, nil
}

func (s *BotServer) sendToConvs(convs []kbchat.Conversation, outputRes string) error {
	outputRes = fmt.Sprintf("```%s```", strings.Replace(outputRes, "\n", "\\n", -1))
	for _, conv := range convs {
		if err := s.kbc.SendMessage(conv.Id, outputRes); err != nil {
			return err
		}
	}
	return nil
}

func (s *BotServer) once() error {

	convs, err := s.kbc.GetConversations(true)
	if err != nil {
		return err
	}

	convs, err = s.getConvsToSend(convs)
	if err != nil {
		return err
	}

	if len(convs) > 0 {
		var ecsInfo bytes.Buffer
		if err := s.runServiceOutput(&ecsInfo); err != nil {
			return err
		}
		if err := s.sendToConvs(convs, ecsInfo.String()); err != nil {
			return err
		}
	}
	return nil
}

func (s *BotServer) Start() error {

	// Start up KB chat
	p := exec.Command(s.opts.KeybaseLocation, "chat", "api")
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

	boutput := bufio.NewScanner(output)
	s.kbc = kbchat.NewAPI(input, boutput)
	s.once()

	for {
		select {
		case <-time.After(2 * time.Second):
			if err := s.once(); err != nil {
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

	bs := NewBotServer(opts, ecs)
	if err := bs.Start(); err != nil {
		fmt.Printf("error running chat loop: %s\n", err.Error())
	}

	return 0
}
