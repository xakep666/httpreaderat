//
// This example can list files inside archive or get a single file from it without
// downloading the whole archive. If the server does not support HTTP Range
// Requests, the whole file is downloaded to a backing store as a fallback.
//
package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/avvmoto/buf-readerat"

	"github.com/xakep666/httpreaderat"
)

var (
	urlFlag  = flag.String("url", "", "Remote zip archive url")
	fileFlag = flag.String("file", "", "Path to file inside zip archive")
	listFlag = flag.Bool("list", false, "Get list of files inside archive")
)

func openRemoteZip(url string) (*zip.Reader, error) {
	// create http.Request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("http request create failed: %w", err)
	}

	// a backing store in case the server does not support range requests
	bs := httpreaderat.NewDefaultStore()
	defer bs.Close()

	// make a HTTPReaderAt client
	htrdr, err := httpreaderat.New(nil, req, bs)
	if err != nil {
		return nil, fmt.Errorf("httpreaderat open failed: %w", err)
	}

	// make it buffered
	bhtrdr := bufra.NewBufReaderAt(htrdr, 1024*1024)

	// make a ZIP file reader
	zrdr, err := zip.NewReader(bhtrdr, htrdr.Size())
	if err != nil {
		return nil, fmt.Errorf("zip reader create failed: %w", err)
	}

	return zrdr, nil
}

func printArchiveFiles(archive *zip.Reader) {
	if archive.Comment != "" {
		fmt.Println("Comment:", archive.Comment)
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.TabIndent|tabwriter.Debug)
	defer tw.Flush()

	// heading
	header := []string{
		"Mode",
		"CRC32",
		"Compressed Size",
		"Uncompressed Size",
		"Modification Time",
		"Name",
		"Comment",
	}
	fmt.Fprintln(tw, strings.Join(header, "\t")+"\t")
	sort.Slice(archive.File, func(i, j int) bool {
		return archive.File[i].Name < archive.File[j].Name
	})

	for _, file := range archive.File {
		info := []string{
			file.Mode().String(),
			fmt.Sprintf("0x%x", file.CRC32),
			strconv.FormatUint(file.CompressedSize64, 10),
			strconv.FormatUint(file.UncompressedSize64, 10),
			file.Modified.Format(time.Stamp + " 2006"),
			file.Name,
			file.Comment,
		}

		fmt.Fprintln(tw, strings.Join(info, "\t")+"\t")
	}
}

func catArchiveFile(archive *zip.Reader, path string) {
	for _, file := range archive.File {
		if file.Name == path {
			rc, err := file.Open()
			if err != nil {
				fmt.Println("File open failed:", err)
				os.Exit(2)
			}

			_, err = io.Copy(os.Stdout, rc)
			rc.Close()

			if err != nil {
				fmt.Println("File read failed:", err)
				os.Exit(2)
			}

			return
		}
	}

	fmt.Println("No such file in archive, try to use -list first to see files")
	os.Exit(3)
}

func main() {
	flag.Parse()
	if urlFlag == nil {
		flag.Usage()
		fmt.Println("ZIP archive url is required")
		os.Exit(1)
	}

	archive, err := openRemoteZip(*urlFlag)
	if err != nil {
		flag.Usage()
		fmt.Println("Failed to open remote archive:", err)
		os.Exit(2)
	}

	if *listFlag {
		printArchiveFiles(archive)
	} else {
		if fileFlag == nil {
			flag.Usage()
			fmt.Println("File path is required")
			os.Exit(1)
		}

		catArchiveFile(archive, *fileFlag)
	}
}
