package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func exportCommand() *cobra.Command {
	var (
		id                int64
		chatcmpl          string
		requestID         string
		output            string
		directory         string
		escapeHTML        bool
		goodCase, badCase bool
		tags              []string
		curl              bool
	)
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export a Moonshot AI request",
		Run: func(cmd *cobra.Command, args []string) {
			request, err := persistence.GetRequest(id, chatcmpl, requestID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					logFatal(sql.ErrNoRows)
				}
				logFatal(err)
			}
			if curl {
				if err := writeCurlCommand(os.Stdout, request); err != nil {
					logFatal(err)
				}
				return
			}
			if request.IsChat() {
				switch {
				case goodCase:
					request.Category = "goodcase"
				case badCase:
					request.Category = "badcase"
				}
				if len(tags) > 0 {
					request.Tags = tags
				}
			}
			var outputStream io.Writer
			if directory != "" {
				var file *os.File
				file, err = os.Create(filepath.Join(directory, genFilename(request)))
				if err != nil {
					logFatal(err)
				}
				defer file.Close()
				outputStream = file
			} else {
				switch output {
				case "stdout":
					outputStream = os.Stdout
				case "stderr":
					outputStream = os.Stderr
				default:
					file, err := os.Create(output)
					if err != nil {
						logFatal(err)
					}
					defer file.Close()
					outputStream = file
				}
			}
			encoder := json.NewEncoder(outputStream)
			encoder.SetIndent("", "    ")
			encoder.SetEscapeHTML(escapeHTML)
			if err = encoder.Encode(request); err != nil {
				logFatal(err)
			}
		},
	}
	flags := cmd.PersistentFlags()
	flags.Int64Var(&id, "id", 0, "row id")
	flags.StringVar(&chatcmpl, "chatcmpl", "", "chatcmpl")
	flags.StringVar(&requestID, "requestid", "", "request id returned from Moonshot AI")
	flags.StringVarP(&output, "output", "o", "stdout", "output file path")
	flags.StringVar(&directory, "directory", "", "output directory")
	flags.BoolVar(&escapeHTML, "escape-html", false, "specifies whether problematic HTML characters should be escaped")
	flags.BoolVar(&goodCase, "good", false, "good case")
	flags.BoolVar(&badCase, "bad", false, "bad case")
	flags.StringArrayVar(&tags, "tag", nil, "tags describe the current case")
	flags.BoolVar(&curl, "curl", false, "export curl command")
	cmd.MarkFlagsOneRequired("id", "chatcmpl", "requestid")
	cmd.MarkFlagsMutuallyExclusive("good", "bad")
	cmd.MarkPersistentFlagFilename("output")
	cmd.MarkPersistentFlagDirname("directory")
	return cmd
}

func genFilename(request *Request) (filename string) {
	if ident := request.Ident(); strings.HasPrefix(ident, "chatcmpl=") {
		filename = strings.TrimPrefix(ident, "chatcmpl=") + ".json"
	} else if strings.HasPrefix(ident, "requestid=") {
		filename = "requestid-" + strings.TrimPrefix(ident, "requestid=") + ".json"
	} else {
		var filenameBuilder strings.Builder
		filenameBuilder.WriteString(strings.ToLower(request.RequestMethod))
		filenameBuilder.WriteString("-")
		filenameBuilder.WriteString(
			strings.ReplaceAll(
				strings.TrimPrefix(request.RequestPath, "/v1/"),
				"/", "-"),
		)
		if request.MoonshotUID.Valid {
			filenameBuilder.WriteString("-")
			filenameBuilder.WriteString(request.MoonshotUID.String)
		}
		filenameBuilder.WriteString("-")
		filenameBuilder.WriteString(request.CreatedAt.Format("20060102150405"))
		filename = filenameBuilder.String() + ".json"
	}
	return filename
}

func writeCurlCommand(w io.Writer, request *Request) error {
	escape := func(s string) string {
		return strings.ReplaceAll(s, "'", `'"'"'`)
	}
	if _, err := io.WriteString(w,
		"curl -X '"+
			escape(request.RequestMethod)+
			"' '"+
			escape(request.Url())+
			"' \\\n\t",
	); err != nil {
		return err
	}
	if _, err := io.WriteString(w,
		`-H "Authorization: Bearer $MOONSHOT_API_KEY"`+"\\\n\t",
	); err != nil {
		return err
	}
	if request.RequestHeader.Valid {
		mimeHeader, _ := textproto.
			NewReader(bufio.NewReader(strings.NewReader(request.RequestHeader.String + "\r\n\r\n"))).
			ReadMIMEHeader()
		mimeHeader.Del("Content-Length")
		mimeHeader.Del("X-Unix-Micro")
		for k, vv := range mimeHeader {
			for _, v := range vv {
				if _, err := io.WriteString(w,
					"-H '"+
						escape(k)+
						": "+
						escape(v)+
						"' \\\n\t",
				); err != nil {
					return err
				}
			}
		}
	}
	if request.RequestBody.Valid {
		if _, err := io.WriteString(w,
			"-d '"+
				escape(request.RequestBody.String)+
				"'",
		); err != nil {
			return err
		}
	}
	if _, err := w.Write([]byte("\n")); err != nil {
		return err
	}
	return nil
}
