package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type SippyTest struct {
	Name             string `json:"name"`
	JiraComponent    string `json:"jira_component"`
	CurrentRuns      int    `json:"current_runs"`
	CurrentSuccesses int    `json:"current_successes"`
	CurrentFlakes    int    `json:"current_flakes"`
	CurrentFailures  int    `json:"current_failures"`

	version   string
	namespace string
}

type SippyTestOutput struct {
	Url    string `json:"url"`
	Output string `json:"output"`
}

type SippyTestOutputProcessed struct {
	Url     string
	Outputs []string
}

const (
	v422 = "4.22"

	currentVersion = v422

	sippyFilter = `{"items":[{"columnField":"name","operatorValue":"starts with","value":"[Monitor:no-default-service-account-operator-checker][sig-auth] all pods in"},{"columnField":"name","operatorValue":"ends with","value":"namespace must not use the default service account."}],"linkOperator":"and"}`
)

var (
	out = os.Stdout

	versions = []string{v422}
)

func main() {

	// first argument is the filename to write output to (defaults to STDOUT)
	if len(os.Args) > 1 {
		var err error
		out, err = os.Create(os.Args[1])
		if err != nil {
			panic(err)
		}
	}

	fmt.Println("\nretrieving sippy tests")

	removeExceptionsList := []*SippyTestOutputProcessed{}

	for _, v := range versions {
		sippyTests := sippyTests(v)
		sippyTestOutputs := sippyTestOutputs(sippyTests)
		sippyTestOutputsProcessed := sippyTestOutputsProcessed(sippyTestOutputs)
		removeExceptionsList = removeExceptions(sippyTestOutputsProcessed)
	}
	fmt.Printf("found %d tests in total without exceptions\n", len(removeExceptionsList))

	// Print Markdown Table
	fmt.Fprintln(out, "| Test URL | Remaining Outputs (No Exceptions) |")
	fmt.Fprintln(out, "| :--- | :--- |")

	for _, item := range removeExceptionsList {
		// Join multiple outputs with <br> so they appear on new lines within the same Markdown cell
		joinedOutputs := strings.Join(item.Outputs, "<br>")

		// Clean up any pipes in the text so they don't break the Markdown table structure
		safeOutputs := strings.ReplaceAll(joinedOutputs, "|", "\\|")

		fmt.Fprintf(out, "| %s | %s |\n", item.Url, safeOutputs)
	}
}

func sippyTests(version string) []*SippyTest {
	sippyReq := fmt.Sprintf("https://sippy.dptools.openshift.org/api/tests?release=%s&filter=%s",
		version,
		url.QueryEscape(sippyFilter),
	)

	resp, err := http.Get(sippyReq)
	if err != nil {
		panic(err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	var tests []*SippyTest
	if err := json.Unmarshal(body, &tests); err != nil {
		panic(err)
	}

	for _, t := range tests {
		t.version = version
		t.namespace = getNamespace(t.Name)
	}

	return tests
}

func sippyTestOutputs(sippyTests []*SippyTest) []*SippyTestOutput {
	monitorTestNames := []string{}
	outputs := [][]*SippyTestOutput{}
	for _, sp := range sippyTests {
		monitorTestNames = append(monitorTestNames, sp.Name)
	}
	for _, name := range monitorTestNames {
		sippyReq := fmt.Sprintf(
			"https://sippy.dptools.openshift.org/api/tests/outputs?release=%s&test=%s",
			currentVersion,
			url.QueryEscape(name),
		)
		resp, err := http.Get(sippyReq)
		if err != nil {
			panic(err)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			panic(err)
		}
		var output []*SippyTestOutput
		if err := json.Unmarshal(body, &output); err != nil {
			panic(err)
		}
		outputs = append(outputs, output)
	}
	outputs1 := []*SippyTestOutput{}
	for i, outputs := range outputs {
		for _, output := range outputs {
			output.Output = monitorTestNames[i] + " - " + output.Output
		}
		outputs1 = append(outputs1, outputs...)
	}
	return outputs1
}

func sippyTestOutputsProcessed(sippyTestOutputs []*SippyTestOutput) []*SippyTestOutputProcessed {
	sippyTestOutputsProcessed := []*SippyTestOutputProcessed{}
	for _, sto := range sippyTestOutputs {
		outputs := strings.Split(sto.Output, "\n")
		sippyTestOutputsProcessed = append(
			sippyTestOutputsProcessed,
			&SippyTestOutputProcessed{
				Url:     sto.Url,
				Outputs: outputs,
			})
	}
	return sippyTestOutputsProcessed
}

func removeExceptions(sippyTestOutputProcessed []*SippyTestOutputProcessed) []*SippyTestOutputProcessed {
	newStop := []*SippyTestOutputProcessed{}
	for _, stop := range sippyTestOutputProcessed {
		newOutputs := []string{}
		for _, output := range stop.Outputs {
			if !strings.Contains(output, "(exception: ") {
				newOutputs = append(newOutputs, output)
			}
		}
		stop.Outputs = newOutputs
		if len(stop.Outputs) > 0 {
			newStop = append(newStop, stop)
		}
	}
	return newStop
}

func getNamespace(testName string) string {
	ns := testName
	ns = strings.ReplaceAll(ns, "[sig-auth] all workloads in ns/", "")
	ns = strings.ReplaceAll(ns, " must set the 'openshift.io/required-scc' annotation", "")
	return ns
}
