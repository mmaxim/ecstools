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

func NewBotServer(opts Options) *BotServer {
	return &BotServer{
		opts: opts,
	}
}

func (s *BotServer) runServiceOutput(cluster string, out io.Writer) error {

	ecs, err := libecs.New(libecs.ECSConfig{
		Cluster: cluster,
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

func (s *BotServer) shouldSendToConv(convID string) (string, error) {

	msgs, err := s.kbc.GetTextMessages(convID, true)
	if err != nil {
		return "", err
	}
	for _, msg := range msgs {
		if msg.Content.Type == "text" && strings.HasPrefix(msg.Content.Text.Body, "!ecslist") {
			toks := strings.Split(msg.Content.Text.Body, " ")
			if len(toks) == 2 {
				return toks[1], nil
			} else if len(toks) == 1 {
				return s.opts.ClusterName, nil
			}
			return "", fmt.Errorf("invalid ecslist command")
		}
	}

	return "", nil
}

type runSpec struct {
	conv    kbchat.Conversation
	cluster string
}

func (s *BotServer) getConvsToSend(convs []kbchat.Conversation) ([]runSpec, error) {
	var res []runSpec
	for _, conv := range convs {
		cluster, err := s.shouldSendToConv(conv.Id)
		if err != nil {
			return nil, err
		}
		if len(cluster) > 0 {
			res = append(res, runSpec{conv: conv, cluster: cluster})
			fmt.Printf("sending to: id: %s name: %s cluster: %s\n", conv.Id, conv.Channel.Name, cluster)
		}
	}
	return res, nil
}

func (s *BotServer) once() error {

	convs, err := s.kbc.GetConversations(true)
	if err != nil {
		return err
	}

	specs, err := s.getConvsToSend(convs)
	if err != nil {
		return err
	}

	if len(specs) > 0 {
		var ecsInfo bytes.Buffer
		for _, spec := range specs {
			if err := s.kbc.SendMessage(spec.conv.Id, fmt.Sprintf("loading cluster: %s", spec.cluster)); err != nil {
				return err
			}
			if err := s.runServiceOutput(spec.cluster, &ecsInfo); err != nil {
				return err
			}
			outputRes := fmt.Sprintf("```%s```", strings.Replace(ecsInfo.String(), "\n", "\\n", -1))
			if err := s.kbc.SendMessage(spec.conv.Id, outputRes); err != nil {
				return err
			}
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

	bs := NewBotServer(opts)
	if err := bs.Start(); err != nil {
		fmt.Printf("error running chat loop: %s\n", err.Error())
	}

	return 0
}
