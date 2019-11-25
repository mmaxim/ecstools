package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"time"

	"bytes"

	"strings"

	"github.com/keybase/go-keybase-chat-bot/kbchat"
	"github.com/keybase/go-keybase-chat-bot/kbchat/types/chat1"
	"github.com/mmaxim/ecstools/libecs"
)

func main() {
	rc := mainInner()
	os.Exit(rc)
}

const trips = "```"

type Options struct {
	ClusterName     string
	TeamName        string
	Region          string
	ShortArns       bool
	KeybaseLocation string
	Home            string
}

type BotServer struct {
	opts Options

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

func (s *BotServer) makeAdvertisement(teamName string) kbchat.Advertisement {
	var listExtendedBody = fmt.Sprintf(`!ecslist by itself will dump out information about the default cluster. By specifying a valid cluster name, a user can get info about it as well. Example usages:
%s!ecslist        # list out information about default cluster
!ecslist kbfs   # list out information about kbfs%s`, trips, trips)
	return kbchat.Advertisement{
		Alias: "AWS ECS Info Bot",
		Advertisements: []chat1.AdvertiseCommandAPIParam{
			{
				Typ: "public",
				Commands: []chat1.UserBotCommandInput{
					{
						Name:        "ecslist",
						Usage:       "[cluster]",
						Description: "List all running tasks and services on an ECS cluster",
						ExtendedDescription: &chat1.UserBotExtendedDescription{
							Title: `*!ecslist* [cluster]
List all running tasks and services on an ECS cluster`,
							DesktopBody: listExtendedBody,
							MobileBody:  listExtendedBody,
						},
					},
					{
						Name:        "ecssvcgraph",
						Usage:       "<cluster> <service> <cpu|mem>",
						Description: "Attach a graph of service perf",
					},
				},
			},
		},
	}
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

	if err := output.DisplayServices(services, out); err != nil {
		s.debug("failed to display: %s", err.Error())
		return err
	}
	return nil
}

func (s *BotServer) handleCommand(conv chat1.ConvSummary, msg chat1.MsgSummary) {
	if msg.Content.Text == nil {
		return
	}
	body := msg.Content.Text.Body
	switch {
	case strings.HasPrefix(body, "!ecslist"):
		s.handleListCommand(conv, msg)
	case strings.HasPrefix(body, "!ecssvcgraph"):
		s.handleGraph(conv, msg)
	}
}

func (s *BotServer) handleGraph(conv chat1.ConvSummary, msg chat1.MsgSummary) (err error) {
	defer func() {
		if err != nil {
			if _, err := s.kbc.ReactByConvID(conv.Id, msg.Id, ":-1:"); err != nil {
				s.debug("failed to react: %s", err)
			}
			s.kbc.SendMessageByConvID(conv.Id, "invalid graph command: %s", err)
		}
	}()
	toks := strings.Split(strings.Trim(msg.Content.Text.Body, " "), " ")
	if len(toks) != 4 {
		return errors.New("wrong number of arguments")
	}
	ecs, err := libecs.New(libecs.ECSConfig{
		Cluster: toks[1],
		Region:  s.opts.Region,
	})
	if err != nil {
		s.debug("failed to create ECS API object: %s", err)
		return err
	}
	var res io.Reader
	switch toks[3] {
	case "cpu":
		res, err = ecs.GetServiceCPUGraph(toks[2], 24*time.Hour)
	case "mem":
		res, err = ecs.GetServiceCPUGraph(toks[2], 24*time.Hour)
	default:
		return errors.New("unknown metric")
	}
	if err != nil {
		return err
	}

	file, err := ioutil.TempFile("", "graph")
	if err != nil {
		return err
	}
	defer file.Close()
	defer os.Remove(file.Name())
	if _, err := io.Copy(file, res); err != nil {
		return err
	}
	if _, err := s.kbc.SendAttachmentByConvID(conv.Id, file.Name(), ""); err != nil {
		return err
	}
	return nil
}

func (s *BotServer) handleListCommand(conv chat1.ConvSummary, msg chat1.MsgSummary) {
	toks := strings.Split(strings.Trim(msg.Content.Text.Body, " "), " ")
	if _, err := s.kbc.ReactByConvID(conv.Id, msg.Id, ":wave:"); err != nil {
		s.debug("failed to react: %s", err)
	}
	var spec *runSpec
	if len(toks) == 2 {
		spec = &runSpec{conv: conv, msg: msg, cluster: toks[1], author: msg.Sender.Username}
	} else if len(toks) == 1 {
		spec = &runSpec{conv: conv, msg: msg, cluster: s.opts.ClusterName, author: msg.Sender.Username}
	} else {
		if _, err := s.kbc.ReactByConvID(conv.Id, msg.Id, ":-1:"); err != nil {
			s.debug("failed to react: %s", err)
		}
		s.kbc.SendMessageByConvID(conv.Id, "invalid ecslist command")
		return
	}
	if err := s.sendReply(spec); err != nil {
		s.debug("failed to send reply: %s", err)
	}
}

type runSpec struct {
	conv    chat1.ConvSummary
	msg     chat1.MsgSummary
	cluster string
	author  string
}

func (s *BotServer) sendReply(spec *runSpec) error {
	var ecsInfo bytes.Buffer
	greet := fmt.Sprintf("Thanks @%s! Loading cluster: *%s*", spec.author, spec.cluster)
	if _, err := s.kbc.SendMessageByConvID(spec.conv.Id, greet); err != nil {
		return err
	}
	if err := s.runServiceOutput(spec.cluster, &ecsInfo); err != nil {
		return err
	}
	outputRes := fmt.Sprintf("```%s```", ecsInfo.String())
	if _, err := s.kbc.SendMessageByConvID(spec.conv.Id, outputRes); err != nil {
		return err
	}
	if _, err := s.kbc.ReactByConvID(spec.conv.Id, spec.msg.Id, ":white_check_mark:"); err != nil {
		s.debug("failed to react: %s", err)
	}
	return nil
}

func (s *BotServer) Start() (err error) {

	// Start up KB chat
	if s.kbc, err = kbchat.Start(kbchat.RunOptions{
		KeybaseLocation: s.opts.KeybaseLocation,
		HomeDir:         s.opts.Home,
	}); err != nil {
		return err
	}
	if _, err := s.kbc.AdvertiseCommands(s.makeAdvertisement(s.opts.TeamName)); err != nil {
		s.debug("advertise error: %s", err)
		return err
	}

	if s.opts.TeamName != "" {
		if _, err := s.kbc.SendMessageByTeamName(s.opts.TeamName, nil, "I'm running."); err != nil {
			s.debug("failed to announce self: %s", err)
			return err
		}
	}
	// Subscribe to new messages
	sub, err := s.kbc.ListenForNewTextMessages()
	if err != nil {
		return err
	}
	s.debug("startup success, listening for messages...")
	for {
		msg, err := sub.Read()
		if err != nil {
			s.debug("Read() error: %s", err.Error())
			continue
		}
		s.handleCommand(msg.Conversation, msg.Message)
	}
}

func mainInner() int {
	var opts Options

	flag.StringVar(&opts.KeybaseLocation, "keybase", "keybase", "keybase command")
	flag.StringVar(&opts.ClusterName, "cluster", "gregord", "cluster name")
	flag.StringVar(&opts.Region, "region", "us-east-1", "AWS region name")
	flag.StringVar(&opts.TeamName, "teamname", "", "Team to operate in")
	flag.StringVar(&opts.Home, "home", "", "Home directory")
	flag.BoolVar(&opts.ShortArns, "short-arns", true, "display only last part of ARN")
	flag.Parse()

	bs := NewBotServer(opts)
	if err := bs.Start(); err != nil {
		fmt.Printf("error running chat loop: %s\n", err.Error())
	}

	return 0
}
