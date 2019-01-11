package libecs

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/reconquest/loreley"
)

type ServiceOutputer interface {
	Display(svcs []Service, w io.Writer) error
}

type BasicServiceOutputer struct {
	shortArns bool
}

func NewBasicServiceOutputer(shortArns bool) BasicServiceOutputer {
	return BasicServiceOutputer{
		shortArns: shortArns,
	}
}

func (o BasicServiceOutputer) truncateARN(arn string, enabled bool) string {
	if !enabled {
		return arn
	}
	toks := strings.Split(arn, "/")
	return toks[1]
}

func (o BasicServiceOutputer) Display(services []Service, out io.Writer) error {
	buffer := &bytes.Buffer{}
	w := tabwriter.NewWriter(buffer, 0, 3, 5, ' ', tabwriter.FilterHTML)
	fmt.Fprintf(w, "Name\tRunning\tPending\tTask\tCPU%%\tMemory%%\n")
	for _, s := range services {
		fmt.Fprintf(w, "%s\t%d\t%d\t%s\t%f\t%f\n", s.Name, s.RunningCount, s.PendingCount,
			o.truncateARN(s.TaskDefinition, o.shortArns), s.Metrics.CPU, s.Metrics.Memory)
	}
	w.Flush()
	fmt.Fprintf(w, "\n")
	w.Flush()

	fmt.Fprintf(w, "Service\tStatus\tDesired\tTask\tCreated At\tInstance ID\tInstance CPU%%\n")
	for _, svc := range services {
		for _, task := range svc.Tasks {
			ca := task.CreatedAt.Format("01-02-2006 15:04")
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%f%%\n", svc.Name, task.Status, task.DesiredStatus,
				o.truncateARN(task.TaskDefinition, o.shortArns), ca, task.InstanceMetrics.ID,
				task.InstanceMetrics.CPU)
		}
	}
	w.Flush()

	loreley.DelimLeft = "<"
	loreley.DelimRight = ">"
	result, err := loreley.CompileAndExecuteToString(buffer.String(), nil, nil)
	if err != nil {
		return fmt.Errorf("error formating output: %s", err.Error())
	}

	if _, err = out.Write([]byte(result)); err != nil {
		return fmt.Errorf("error writing output: %s", err.Error())
	}

	return nil
}

type ColorServiceOutputer struct {
	shortArns bool
}

func NewColorServiceOutputer(shortArns bool) ColorServiceOutputer {
	return ColorServiceOutputer{
		shortArns: shortArns,
	}
}

func (o ColorServiceOutputer) truncateARN(arn string, enabled bool) string {
	if !enabled {
		return arn
	}
	toks := strings.Split(arn, "/")
	return toks[1]
}

func (o ColorServiceOutputer) Display(services []Service, out io.Writer) error {
	buffer := &bytes.Buffer{}
	w := tabwriter.NewWriter(buffer, 0, 3, 5, ' ', tabwriter.FilterHTML)
	fmt.Fprintf(w, "<fg 13><bold>Name\tRunning\tPending\tTask\tCPU%%\tMemory%%<reset>\n")
	for _, s := range services {
		fmt.Fprintf(w, "%s\t%d\t%d\t%s\t%f\t%f\n", s.Name, s.RunningCount, s.PendingCount,
			o.truncateARN(s.TaskDefinition, o.shortArns), s.Metrics.CPU, s.Metrics.Memory)
	}
	w.Flush()
	fmt.Fprintf(w, "\n")
	w.Flush()

	fmt.Fprintf(w, "<fg 13><bold>Service\tStatus\tDesired\tTask\tCreated At\tInstance ID\tInstance CPU%%<reset>\n")
	for _, svc := range services {
		for _, task := range svc.Tasks {
			ca := task.CreatedAt.Format("01-02-2006 15:04")
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%f%%\n", svc.Name, task.Status, task.DesiredStatus,
				o.truncateARN(task.TaskDefinition, o.shortArns), ca, task.InstanceMetrics.ID,
				task.InstanceMetrics.CPU)
		}
	}
	w.Flush()

	loreley.DelimLeft = "<"
	loreley.DelimRight = ">"
	result, err := loreley.CompileAndExecuteToString(buffer.String(), nil, nil)
	if err != nil {
		return fmt.Errorf("error formating output: %s", err.Error())
	}

	if _, err = out.Write([]byte(result)); err != nil {
		return fmt.Errorf("error writing output: %s", err.Error())
	}

	return nil
}
