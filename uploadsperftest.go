package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	minio "github.com/minio/minio-go"
	// minio "github.com/minio/minio-go"
)

const (
	// constant for default random seed.
	defaultRandomSeed = 42

	// minimum worker running time
	workerDuration = time.Duration(time.Minute * 15)

	// minimum per worker upload count
	minUploadCount = 10
)

var (
	alNum = []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

	// represent parent dirs when spaces are replaced by path
	// separators
	parentDirs = []string{
		"It is certain",
		"It is decidedly so",
		"Without a doubt",
		"Yes definitely",
		"You may rely on it",
		"As I see it yes",
		"Most likely",
		"Outlook good",
		"Yes",
		"Signs point to yes",
		"Reply hazy try again",
		"Ask again later",
		"Better not tell you now",
		"Cannot predict now",
		"Concentrate and ask again",
		"Don't count on it",
		"My reply is no",
		"My sources say no",
		"Outlook not so good",
		"Very doubtful",
	}

	// Read settings from environment
	accessKey = os.Getenv("ACCESS_KEY")
	secretKey = os.Getenv("SECRET_KEY")

	// settings from command line
	endpoint    string
	secure      bool
	bucket      string
	concurrency int
	randomSeed  int64
	outputFile  string
)

// object generator type - generates object content without IO.
type ObjGen struct {
	// name of object
	ObjectName string

	// size in KiB
	ObjectSize int64

	// seed string that repeats inside the object
	SeedBytes []byte

	// number of bytes read
	readCount int64

	// index to read at
	readIndex int
}

func NewRandomObjectWithSize(size int64) ObjGen {
	return ObjGen{
		ObjectName: getRandomObjectName(),
		ObjectSize: size,
		SeedBytes:  []byte(getAlNumPerm()),
	}
}

// implement Reader interface
func (og *ObjGen) Read(p []byte) (n int, err error) {
	for ; n < len(p) && og.readCount < og.ObjectSize; n++ {
		p[n] = og.SeedBytes[og.readIndex]
		og.readIndex = (og.readIndex + 1) % len(og.SeedBytes)
		og.readCount++
	}
	if og.readCount >= og.ObjectSize {
		err = io.EOF
	}
	return
}

// returns length of object
func (og *ObjGen) Size() int64 {
	return og.ObjectSize
}

func getAlNumPerm() string {
	n := len(alNum)
	p := rand.Perm(n)
	objNameRunes := make([]rune, n)
	for i := 0; i < n; i++ {
		objNameRunes[i] = alNum[p[i]]
	}
	return string(objNameRunes)
}

func getRandomObjectName() string {
	dirString := parentDirs[rand.Intn(len(parentDirs))]
	objPath := filepath.Join(strings.Fields(dirString)...)

	pStr := getAlNumPerm()
	n := 1 + rand.Intn(len(pStr))

	return filepath.Join(objPath, pStr[:n])
}

// Returns number of bytes expressed by human friendly
// string. Supports:
//
// 1. Raw byte number ("124")
// 2. Number with unit (no intervening whitespace).
//
// Supported units: KB, MB, GB, TB, KiB, MiB, GiB and TiB.
func parseHumanNumber(s string) (int64, error) {
	multiplier := []int64{
		1000,
		1000 * 1000,
		1000 * 1000 * 1000,
		1000 * 1000 * 1000 * 1000,
		1024,
		1024 * 1024,
		1024 * 1024 * 1024,
		1024 * 1024 * 1024 * 1024,
	}
	suffixes := []string{
		"KB", "MB", "GB", "TB",
		"KiB", "MiB", "GiB", "TiB",
	}
	badSizeErr := errors.New("invalid size number given")
	for i, suffix := range suffixes {
		if strings.HasSuffix(s, suffix) {
			v := strings.TrimSuffix(s, suffix)
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return 0, badSizeErr
			}
			return n * multiplier[i], nil
		}
	}
	// try to parse raw byte number
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, badSizeErr
	}
	return n, nil
}

var (
	errWorkerSucc = errors.New("Worker is exiting with success.")
	errWorkerQuit = errors.New("Worker is quitting due to quit signal.")
)

type workerMsg struct {
	// If exitingErr != nil -> worker is quitting with an error
	// value.
	exitingErr error

	// Sends time at which putobject was successful
	putStartTime time.Time
	putDuration  time.Duration
}

func workerLoop(objSize int64, workerMsgCh chan<- workerMsg, quitChan <-chan struct{}) {
	mc, err := minio.New(endpoint, accessKey, secretKey, secure)
	if err != nil {
		workerMsgCh <- workerMsg{exitingErr: err}
		return
	}

	uploader := func(doneCh chan<- workerMsg) {
		object := NewRandomObjectWithSize(objSize)
		startTime := time.Now().UTC()
		_, err := mc.PutObject(bucket, object.ObjectName, &object,
			"")
		duration := time.Since(startTime)
		doneCh <- workerMsg{err, startTime, duration}
	}

	// buffered channel so that uploader go routine does not hang.
	doneCh := make(chan workerMsg, 1)
	uploadCount := 0
	timeStart := time.Now().UTC()
	go uploader(doneCh)
	toQuit := false
	for !toQuit {
		select {
		case uploadMsg := <-doneCh:
			workerMsgCh <- uploadMsg
			if uploadMsg.exitingErr != nil {
				toQuit = true
			} else {
				uploadCount++
				if time.Since(timeStart) < workerDuration ||
					uploadCount < minUploadCount {
					go uploader(doneCh)
				} else {
					workerMsgCh <- workerMsg{
						exitingErr: errWorkerSucc,
					}
					toQuit = true
				}
			}
		case <-quitChan:
			workerMsgCh <- workerMsg{
				exitingErr: errWorkerQuit,
			}
			toQuit = true
		}
	}
}

type TestResult struct {
	putStartTime []time.Time
	putDuration  []time.Duration

	secondCount map[time.Time]int
}

func NewTestResult() TestResult {
	numSeconds := int(workerDuration.Seconds())
	return TestResult{
		putStartTime: make([]time.Time, 0, numSeconds*10),
		putDuration:  make([]time.Duration, 0, numSeconds*10),
		secondCount:  make(map[time.Time]int, numSeconds),
	}
}

func writeCSVOutputFile(tr TestResult, outFile string) error {
	f, err := os.Create(outFile)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(f, "startTimeUnixNano,DurationNano")
	if err != nil {
		return err
	}

	for i, startTime := range tr.putStartTime {
		_, err = fmt.Fprintf(f, "%v,%v\n", startTime.UnixNano(),
			tr.putDuration[i].Nanoseconds())
		if err != nil {
			return err
		}
	}

	return f.Close()
}

func printRoutine(msgCh chan string, printerDoneCh chan struct{}) {
	for msg := range msgCh {
		fmt.Print(msg)
	}
	printerDoneCh <- struct{}{}
}

func launchTest(objSize int64) (TestResult, error) {
	workerMsgCh := make(chan workerMsg)

	// channels to print asynch.
	// buffer upto 100 messages
	printMsgCh := make(chan string, 100)
	printerDoneCh := make(chan struct{})
	go printRoutine(printMsgCh, printerDoneCh)

	// quitCh is buffered as some workers may have quit due to
	// errors when we send the quit signal.
	quitCh := make(chan struct{}, concurrency)

	// Start workers
	for i := 0; i < concurrency; i++ {
		go workerLoop(objSize, workerMsgCh, quitCh)
	}

	tRes := NewTestResult()
	// collect results and wait for workers to quit.
	numWorkersQuit := 0
	isQuitting := false
	eachSecond := time.After(time.Second)
	startTime := time.Now().UTC().Round(time.Second)
	var hadUploadError error
	for numWorkersQuit < concurrency {
		select {
		case wMsg := <-workerMsgCh:
			switch {
			case wMsg.exitingErr == errWorkerSucc:
				fallthrough
			case wMsg.exitingErr == errWorkerQuit:
				numWorkersQuit++
			case wMsg.exitingErr != nil:
				fmt.Printf("An upload attempt errored with \"%v\" - aborting test!\n", wMsg.exitingErr)
				hadUploadError = wMsg.exitingErr
				numWorkersQuit++
				if !isQuitting {
					isQuitting = true
					for i := 0; i < concurrency; i++ {
						quitCh <- struct{}{}
					}
				}
			default:
				// got a successful upload msg.
				tRes.putStartTime = append(tRes.putStartTime, wMsg.putStartTime)
				tRes.putDuration = append(tRes.putDuration, wMsg.putDuration)

				putEndTime := wMsg.putStartTime.Add(wMsg.putDuration)
				uploadTime := putEndTime.Round(time.Second)
				tRes.secondCount[uploadTime]++
			}

		// print messages about the running test each second.
		case <-eachSecond:
			t := time.Now().UTC().Round(time.Second).Add(-1 * time.Second)
			// print via a separate go routine so as to
			// not block the for loop for printing.
			printMsgCh <- fmt.Sprintf("%v: %v\n", t.Sub(startTime),
				tRes.secondCount[t])
			eachSecond = time.After(time.Second)
		}
	}

	// Close and confirm the printing channel exits.
	close(printMsgCh)
	<-printerDoneCh

	return tRes, hadUploadError
}

/*

Worker Algo:

1. Generate objects of specified size, and upload sequentially to
service.

2. Report each success via a channel

3. Terminate on:
   a. Error, or
   b. 15 minutes pass and at least 50 objects are uploaded.
   c. Receiving signal to quit.

In the main thread, setup required number of worker threads, and:

1. Record each success and calculate objects/second metric.

2. On error, signal all workers to quit.

3. Wait for threads to quit, and report metrics.

*/

func init() {
	flag.StringVar(&endpoint, "h", "localhost:9000", "service endpoint host")
	flag.BoolVar(&secure, "s", false, "Set if endpoint requires https")
	flag.StringVar(&bucket, "bucket", "bucket", "Bucket to use for uploads test")
	flag.IntVar(&concurrency, "c", 1, "concurrency - number of parallel uploads")
	flag.Int64Var(&randomSeed, "seed", defaultRandomSeed, "random seed")
	flag.StringVar(&outputFile, "o", "output.csv", "CSV formatted output filename")
}

func main() {
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Println("Usage: ./minio-perftest [flags] UPLOADS_SIZE")
		os.Exit(1)
	}

	// parse command line argument
	size, err := parseHumanNumber(flag.Arg(0))
	if err != nil {
		fmt.Println("Usage: ./minio-perftest [flags] UPLOADS_SIZE")
		fmt.Println("\nUPLOADS_SIZE examples: 100, 1MB, 10KiB, etc")
		os.Exit(1)
	}

	// set random seed for this run
	rand.Seed(randomSeed)

	// launch test
	result, err := launchTest(size)
	if err != nil {
		fmt.Println("Quit due to errors.")
		os.Exit(1)
	}

	// write output to CSV file.
	err = writeCSVOutputFile(result, outputFile)
	if err != nil {
		fmt.Printf("Error writing output file: %v\n", err)
		os.Exit(1)
	}
}
