package main

import (
	"flag"
	"fmt"
	"io"
	"os"

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

func (s *BotServer) debug(msg string, args ...interface{}) {
	fmt.Printf("BotServer: "+msg+"\n", args...)
}

func (s *BotServer) runServiceOutput(cluster string, out io.Writer) error {

	ecs, err := libecs.New(libecs.ECSConfig{
		Cluster: cluster,
		Region:  s.opts.Region,
	})
	if err != nil {
		s.debug("failed to create ECS API object: %s", err.Error())
		return err
	}

	services, err := ecs.ListServices()
	if err != nil {
		s.debug("failed to list services: %s", err.Error())
		return err
	}

	output := libecs.NewBasicServiceOutputer(s.opts.ShortArns)

	if err := output.Display(services, out); err != nil {
		s.debug("failed to display: %s", err.Error())
		return err
	}
	return nil
}

func (s *BotServer) shouldSendToConv(conv kbchat.Conversation, msg kbchat.Message) (*runSpec, error) {
	if strings.HasPrefix(msg.Content.Text.Body, "!ecslist") {
		toks := strings.Split(msg.Content.Text.Body, " ")
		if len(toks) == 2 {
			return &runSpec{conv: conv, cluster: toks[1], author: msg.Sender.Username}, nil
		} else if len(toks) == 1 {
			return &runSpec{conv: conv, cluster: s.opts.ClusterName, author: msg.Sender.Username}, nil
		}
		s.kbc.SendMessage(conv.Id, "invalid ecslist command")
		return nil, nil
	}

	return nil, nil
}

type runSpec struct {
	conv    kbchat.Conversation
	cluster string
	author  string
}

func (s *BotServer) sendReply(spec *runSpec) error {
	var ecsInfo bytes.Buffer
	greet := fmt.Sprintf("Thanks %s! Loading cluster: *%s*", spec.author, spec.cluster)
	if err := s.kbc.SendMessage(spec.conv.Id, greet); err != nil {
		return err
	}
	if err := s.runServiceOutput(spec.cluster, &ecsInfo); err != nil {
		return err
	}
	outputRes := fmt.Sprintf("```%s```", strings.Replace(ecsInfo.String(), "\n", "\\n", -1))
	if err := s.kbc.SendMessage(spec.conv.Id, outputRes); err != nil {
		return err
	}
	return nil
}

func (s *BotServer) Start() (err error) {

	// Start up KB chat
	if s.kbc, err = kbchat.NewAPI(s.opts.KeybaseLocation); err != nil {
		return err
	}

	// Subscribe to new messages
	sub := s.kbc.ListenForNewTextMessages()
	for {
		msg, err := sub.Read()
		if err != nil {
			s.debug("Read() error: %s", err.Error())
			continue
		}
		spec, err := s.shouldSendToConv(msg.Conversation, msg.Message)
		if err != nil {
			s.debug("shouldSendToConv() error: %s", err.Error())
			continue
		}

		if spec != nil {
			s.debug("sending reply on conv: %s author: %s cluster: %s", spec.conv.Channel.Name,
				spec.author, spec.cluster)
			s.sendReply(spec)
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
