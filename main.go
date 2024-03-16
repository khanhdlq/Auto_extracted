package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/kjk/lzmadec"
	"github.com/liamg/magic"
	"github.com/radovskyb/watcher"
)

func Unzip(file, dest string) error {
	reader, err := zip.OpenReader(file)
	if err != nil {
		return err
	}
	defer reader.Close()

	// Create a channel to receive signals when each extraction is complete.
	done := make(chan struct{})

	// Create a counter to keep track of how many extractions are completed.
	var extractionCounter sync.WaitGroup
	extractionCounter.Add(len(reader.File))

	// Limit the number of goroutines to avoid exhausting system resources.
	concurrencyLimit := make(chan struct{}, 10) // Adjust the number as needed.

	for _, file := range reader.File {
		// Start a goroutine for each file extraction.
		go func(file *zip.File) {
			// Release the semaphore after processing.
			defer func() {
				extractionCounter.Done()
				<-concurrencyLimit
			}()

			// Acquire the semaphore to limit concurrency.
			concurrencyLimit <- struct{}{}

			filePath := filepath.Join(dest, file.Name)
			if file.FileInfo().IsDir() {
				os.MkdirAll(filePath, os.ModePerm)
				return
			}

			// Create parent directories if they don't exist.
			if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
				log.Println("Error creating directories:", err)
				return
			}
			// Open destination file for writing.
			extractedFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
			if err != nil {
				log.Println("Error opening destination file:", err)
				return
			}
			defer extractedFile.Close()

			// Open file in zip for reading.
			zippedFile, err := file.Open()
			if err != nil {
				log.Println("Error opening zipped file:", err)
				return
			}
			defer zippedFile.Close()

			// Copy data from zipped file to extracted file.
			if _, err := io.Copy(extractedFile, zippedFile); err != nil {
				log.Println("Error copying data:", err)
				return
			}

			// Signal that this extraction is complete.
			done <- struct{}{}
		}(file)
	}

	// Wait for all extractions to be completed.
	go func() {
		extractionCounter.Wait()
		// Close the channel to signal that all extractions are done.
		close(done)
	}()

	// Wait for all extractions to be completed before returning.
	for range reader.File {
		<-done
	}

	return nil
} // =========================================================================================================== Extract zip
func Unrar(path string, dest string) {
	// Check if the RAR file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		log.Fatalf("RAR file '%s' not found", path)
	}
	// Create the destination directory if it doesn't exist
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		if err := os.Mkdir(dest, 0755); err != nil {
			log.Fatalf("Failed to create directory: %v", err)
		}
	}

	// Execute unrar command
	cmd := exec.Command("unrar", "x", path, dest)
	err := cmd.Run()
	if err != nil {
		log.Fatalf("Failed to unrar: %v", err)
	} else {
		color.Blue("[-] Extracted successfully")
	}
} // =========================================================================================================== Extract Rar
func Extract_7z(path string, dstDir string) error {
	a, err := lzmadec.NewArchive(path)
	if err != nil {
		return err
	}
	for _, e := range a.Entries {
		err = a.ExtractToFile(dstDir+"/"+e.Path, e.Path)
		if err != nil {
			return err
		}
	}
	return nil
} // =========================================================================================================== Extract 7z
func ExtractTarGz(path, dest string) error {
	r, err := os.Open(path)
	if err != nil {
		return err
	}
	defer r.Close()

	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dest, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(target, os.FileMode(header.Mode))
		case tar.TypeReg:
			err = os.MkdirAll(filepath.Dir(target), os.ModeDir)
			if err == nil {
				out, err := os.Create(target)
				if err == nil {
					_, err = io.Copy(out, tr)
					out.Close()
				}
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
} // =========================================================================================================== Extract Tar
func checkFileType(path string) string {
	file, err := os.Open(path) // Open the file
	if err != nil {
		fmt.Println("Error:", err)
		return ""
	}
	defer file.Close()

	buffer := make([]byte, 512)
	_, err = io.ReadFull(file, buffer) // Read the first 512 bytes from the file
	if err != nil {
		fmt.Println("Error:", err)
		return ""
	}

	fileType, err := magic.Lookup(buffer) // Lookup file type
	if err != nil {
		if err == magic.ErrUnknown {
			fmt.Println("File type is unknown")
			return ""
		} else {
			panic(err)
		}
	}
	return fileType.Description
}

func processDir(path string, info os.FileInfo) {
	files, err := ioutil.ReadDir(path)
	if err != nil {
		panic(err)
	}

	if len(files) != 0 {
		return
	}

	err = os.Remove(path)
	if err != nil {
		panic(err)
	}
	// log.Print(path, "removed!")
}
func deleteEmptyFolder(folder string) {
	for true {
		time.Sleep(time.Second)
		err := filepath.Walk(folder, func(path string, info os.FileInfo, err error) error {
			if path == folder {
				return nil
			}
			if err != nil {
				log.Printf("prevent panic by handling failure accessing a path %q: %v\n", path, err)
				return err
			}
			if info.IsDir() {
				processDir(path, info)
			}

			return nil
		})
		if err != nil {
			panic(err)
			return
		}
	}
}
func extract(file string, dest string) {
	contentType := checkFileType(file)
	color.Green("[+] Processing: " + file)

	switch {
	case strings.Contains(contentType, "zip file format"):
		if err := Unzip(file, dest); err != nil {
			log.Println("Error extracting zip:", err)
		} else {
			color.Blue("[-] Extracted successfully")
		}
	case strings.Contains(contentType, "RAR archive"):
		Unrar(file, dest)
	case contentType == "GZIP compressed file":
		if err := ExtractTarGz(file, dest); err != nil {
			log.Println("Error extracting tar:", err)
		} else {
			color.Blue("[-] Extracted successfully")
		}
	case contentType == "7-Zip File Format":
		if err := Extract_7z(file, dest); err != nil {
			log.Println("Error extracting 7z:", err)
		} else {
			color.Blue("[-] Extracted successfully")
		}
	case strings.Contains(file, ".txt"):
		err := os.Rename(file, dest+"/"+filepath.Base(file))
		if err != nil {
			log.Println(err)
			return
		} else {
			color.Blue("[-] Moved successfully")
		}
	default:
		color.Red("[-] Invalid format")
	}
	fmt.Println()

	// Remove the source file
	if err := os.RemoveAll(file); err != nil {
		log.Print(err)
	}
} // =========================================================================================================== Extract all files
func walkDir(fileName string) {
	var files []string
	err := filepath.Walk(fileName, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == fileName {
			return nil
		}
		allowed := map[string]bool{
			"zip file format":      true,
			"RAR archive":          true,
			"GZIP compressed file": true,
			"7-Zip File Format":    true,
		}

		if _, ok := allowed[checkFileType(path)]; ok || strings.Contains(path, ".txt") {
			files = append(files, path)
		} else {
			color.Red("Not allowed " + path)
			err = os.RemoveAll(path)
		}
		return nil
	})

	if err != nil {
		log.Println(err)
		return
	}

	for _, file := range files {
		extract(file, "./leaked")
	}
}

func main() {
	walkDir("./files")
	go deleteEmptyFolder("./files/")
	// ======================================================================================================= Walk first
	color.Yellow("[+] Starting watcher -> [ ./files ]")
	w := watcher.New()

	go func() {
		for {
			select {
			case event := <-w.Event:
				// log.Println(event) // Print the event's info.
				// Check if the event is a create event
				if event.Op == watcher.Create {
					// Get the file info for the created file
					fileInfo, err := os.Stat(event.Path)
					if err != nil {
						log.Println(err)
						continue
					}
					// Check if it's a regular file
					if fileInfo.Mode().IsRegular() {
						// Handle here
						extract(event.Path, "./leaked")
						time.Sleep(time.Millisecond * 4000)
					}
				}
			case err := <-w.Error:
				log.Println(err)
			case <-w.Closed:
				return
			}
		}
	}()
	// Watch test_folder recursively for changes.
	if err := w.AddRecursive("./files"); err != nil {
		log.Println(err)
	}
	for path, file := range w.WatchedFiles() {
		log.Printf("%s: %s\n", path, file.Name())

	}
	log.Println()

	// Trigger 2 events after watcher started.
	go func() {
		w.Wait()
		w.TriggerEvent(watcher.Create, nil)
		w.TriggerEvent(watcher.Remove, nil)
	}()

	// Start the watching process - it'll check for changes every 100ms.
	if err := w.Start(time.Millisecond * 100); err != nil {
		log.Println(err)
	}

}
