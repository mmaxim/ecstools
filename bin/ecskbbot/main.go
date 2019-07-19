package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"bytes"

	"strings"

	"github.com/keybase/go-keybase-chat-bot/kbchat"
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

func (s *BotServer) makeAdvertisement(teamName string) kbchat.Advertisement {
	var listExtendedBody = fmt.Sprintf(`!ecslist by itself will dump out information about the gregord cluster. By specifying a valid cluster name, a user can get info about it as well. Example usages:
%s!ecslist        # list out information about gregord
!ecslist kbfs   # list out information about kbfs%s`, trips, trips)
	return kbchat.Advertisement{
		Alias: "AWS ECS Info Bot",
		Advertisements: []kbchat.CommandsAdvertisement{
			kbchat.CommandsAdvertisement{
				Typ:      "teammembers",
				TeamName: teamName,
				Commands: []kbchat.Command{
					kbchat.Command{
						Name:        "ecslist",
						Usage:       "[core|gregord|kbfs]",
						Description: "List all running tasks and services on an ECS cluster",
						ExtendedDescription: &kbchat.CommandExtendedDescription{
							Title: `*!ecslist* [core|gregord|kbfs]
List all running tasks and services on an ECS cluster`,
							DesktopBody: listExtendedBody,
							MobileBody:  listExtendedBody,
						},
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

	if err := output.Display(services, out); err != nil {
		s.debug("failed to display: %s", err.Error())
		return err
	}
	return nil
}

func (s *BotServer) shouldSendToConv(conv kbchat.Conversation, msg kbchat.Message) (*runSpec, error) {
	if strings.HasPrefix(msg.Content.Text.Body, "!ecslist") {
		toks := strings.Split(strings.Trim(msg.Content.Text.Body, " "), " ")
		if err := s.kbc.ReactByConvID(conv.ID, msg.MsgID, ":wave:"); err != nil {
			s.debug("failed to react: %s", err)
		}
		if len(toks) == 2 {
			return &runSpec{conv: conv, msg: msg, cluster: toks[1], author: msg.Sender.Username}, nil
		} else if len(toks) == 1 {
			return &runSpec{conv: conv, msg: msg, cluster: s.opts.ClusterName, author: msg.Sender.Username}, nil
		}
		if err := s.kbc.ReactByConvID(conv.ID, msg.MsgID, ":-1:"); err != nil {
			s.debug("failed to react: %s", err)
		}
		s.kbc.SendMessageByConvID(conv.ID, "invalid ecslist command")
		return nil, nil
	}

	return nil, nil
}

type runSpec struct {
	conv    kbchat.Conversation
	msg     kbchat.Message
	cluster string
	author  string
}

func (s *BotServer) sendReply(spec *runSpec) error {
	var ecsInfo bytes.Buffer
	greet := fmt.Sprintf("Thanks @%s! Loading cluster: *%s*", spec.author, spec.cluster)
	if err := s.kbc.SendMessageByConvID(spec.conv.ID, greet); err != nil {
		return err
	}
	if err := s.runServiceOutput(spec.cluster, &ecsInfo); err != nil {
		return err
	}
	outputRes := fmt.Sprintf("```%s```", ecsInfo.String())
	if err := s.kbc.SendMessageByConvID(spec.conv.ID, outputRes); err != nil {
		return err
	}
	if err := s.kbc.ReactByConvID(spec.conv.ID, spec.msg.MsgID, ":white_check_mark:"); err != nil {
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
	if err := s.kbc.AdvertiseCommands(s.makeAdvertisement(s.opts.TeamName)); err != nil {
		s.debug("advertise error: %s", err)
		return err
	}

	if err := s.kbc.SendMessageByTeamName(s.opts.TeamName, "I'm running.", nil); err != nil {
		s.debug("failed to announce self: %s", err)
		return err
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
		spec, err := s.shouldSendToConv(msg.Conversation, msg.Message)
		if err != nil {
			s.debug("shouldSendToConv() error: %s", err.Error())
			continue
		}

		if spec != nil {
			s.debug("sending reply on conv: %s(%s) author: %s cluster: %s", spec.conv.Channel.Name,
				spec.conv.ID, spec.author, spec.cluster)
			s.sendReply(spec)
		}
	}
}

func mainInner() int {
	var opts Options

	flag.StringVar(&opts.KeybaseLocation, "keybase", "keybase", "keybase command")
	flag.StringVar(&opts.ClusterName, "cluster", "gregord", "cluster name")
	flag.StringVar(&opts.Region, "region", "us-east-1", "AWS region name")
	flag.StringVar(&opts.TeamName, "teamname", "awsmonitor", "Team to operate in")
	flag.StringVar(&opts.Home, "home", "", "Home directory")
	flag.BoolVar(&opts.ShortArns, "short-arns", true, "display only last part of ARN")
	flag.Parse()

	bs := NewBotServer(opts)
	if err := bs.Start(); err != nil {
		fmt.Printf("error running chat loop: %s\n", err.Error())
	}

	return 0
}
