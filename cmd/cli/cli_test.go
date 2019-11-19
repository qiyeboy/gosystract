package cli

import (
	"bytes"
	"testing"

	"errors"

	"github.com/pjbgf/go-test/should"
	"github.com/pjbgf/gosystract/cmd/systract"
)

func TestParseInputValues(t *testing.T) {
	assertThat := func(assumption string, args []string, expected string) {
		should := should.New(t)

		_, actual, _, err := parseInputValues(args)

		should.NotError(err, assumption)
		should.BeEqual(expected, actual, assumption)
	}

	assertThat("should handle template flag", []string{"gosystract", "--template=\"test\"", ""}, "test")
}

func TestRun(t *testing.T) {
	usageMessageTest := "gosystract version INJECTED\nUsage:\ngosystrac [flags] filePath\n\nFlags:\n\t--dumpfile, -d    Handles a dump file instead of go executable.\n\t--template\t  Defines a go template for the results.\n\t\t\t  Example: --template=\"{{- range . }}{{printf \"%d - %s\\n\" .ID .Name}}{{- end}}\"\n"
	assertThat := func(assumption string, args []string,
		stub func() ([]systract.SystemCall, error), expected string, expectedErr error) {

		should := should.New(t)
		gitcommit = "INJECTED"
		var output bytes.Buffer
		var actualErr error

		Run(&output, args, func(source systract.SourceReader) ([]systract.SystemCall, error) {
			return stub()
		}, func(err error) {
			actualErr = err
		})

		actual := output.String()

		should.BeEqual(expectedErr, actualErr, assumption)
		should.BeEqual(expected, actual, assumption)
	}

	assertThat("should show usage when no args", []string{},
		func() ([]systract.SystemCall, error) { return nil, errors.New("invalid systax") },
		usageMessageTest,
		errors.New("invalid syntax"))

	assertThat("should show syscalls found", []string{"gosystract", "filename"},
		func() ([]systract.SystemCall, error) {
			return []systract.SystemCall{{ID: 1, Name: "abc"}, {ID: 2, Name: "def"}}, nil
		},
		"2 system calls found:\n    abc (1)\n    def (2)\n", nil)

	assertThat("should support custom go template for results",
		[]string{"gosystract", "--template=\"{{- range . }}\"{{.Name}}\",{{- end}}\"", "filename"},
		func() ([]systract.SystemCall, error) {
			return []systract.SystemCall{{ID: 1, Name: "abc"}, {ID: 2, Name: "def"}}, nil
		},
		"\"abc\",\"def\",", nil)

	assertThat("should show message when no syscalls are found",
		[]string{"gosystract", "filename"},
		func() ([]systract.SystemCall, error) {
			return []systract.SystemCall{}, nil
		},
		"no systems calls were found\n", nil)

	assertThat("should be able to handle exec files",
		[]string{"gosystract", "filename"},
		func() ([]systract.SystemCall, error) {
			return []systract.SystemCall{}, nil
		},
		"no systems calls were found\n", nil)

	assertThat("should not write results if extract failed",
		[]string{"gosystract", "filename"},
		func() ([]systract.SystemCall, error) {
			return nil, errors.New("could not extract syscalls")
		},
		"",
		errors.New("could not extract syscalls"))

	assertThat("should error for invalid go template syntax",
		[]string{"gosystract", "--template=\"{{$%£}\"", "filename"},
		func() ([]systract.SystemCall, error) {
			return []systract.SystemCall{{ID: 1, Name: "abc"}, {ID: 2, Name: "def"}}, nil
		},
		"",
		errors.New("invalid go template"))

	assertThat("should error for invalid go template syntax",
		[]string{"gosystract", "--template=\"{{.Something}}\"", "filename"},
		func() ([]systract.SystemCall, error) {
			return []systract.SystemCall{{ID: 1, Name: "abc"}, {ID: 2, Name: "def"}}, nil
		},
		"",
		errors.New("invalid go template"))
}

func TestRun_SourceReaders(t *testing.T) {
	assertThat := func(assumption string, args []string, expected interface{}) {
		should := should.New(t)
		var output bytes.Buffer
		var actualErr error

		Run(&output, args, func(source systract.SourceReader) ([]systract.SystemCall, error) {
			should.HaveSameType(expected, source, "should be able to handle dump files")
			return []systract.SystemCall{}, nil
		}, func(err error) {
			actualErr = err
		})

		should.NotError(actualErr, assumption)
	}

	assertThat("should be able to handle exec files",
		[]string{"gosystract", "filename"},
		&systract.ExeReader{})
	assertThat("should be able to handle dump files",
		[]string{"gosystract", "--dumpfile", "filename"},
		&systract.DumpReader{})
}
