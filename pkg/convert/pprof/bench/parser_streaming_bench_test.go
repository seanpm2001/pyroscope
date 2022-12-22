package bench

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"github.com/pyroscope-io/pyroscope/pkg/util/form"
	"io"
	"io/fs"
	"mime/multipart"

	"github.com/pyroscope-io/pyroscope/pkg/convert/pprof"
	"github.com/pyroscope-io/pyroscope/pkg/convert/pprof/streaming"

	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"io/ioutil"
	"log"
	"net/http"

	"testing"
	"time"
)

type testcase struct {
	profile, prev []byte
	config        map[string]*tree.SampleTypeConfig
}

func readCorpus(dir string) []*testcase {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Fatal(err)
	}
	var res []*testcase
	for _, file := range files {
		res = append(res, readCorpusItem(dir, file))
	}
	return res
}
func readCorpusItemI(dir string, i int) *testcase {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Fatal(err)
	}

	return readCorpusItem(dir, files[i])

}

func readCorpusItem(dir string, file fs.FileInfo) *testcase {
	bs, err := ioutil.ReadFile(dir + "/" + file.Name())
	if err != nil {
		panic(err)
	}
	r, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(bs)))
	if err != nil {
		panic(err)
	}
	_ = r
	//fmt.Printf("%s %d\n", file.Name(), len(bs))
	contentType := r.Header.Get("Content-Type")

	rawData, _ := ioutil.ReadAll(r.Body)

	boundary, err := form.ParseBoundary(contentType)
	if err != nil {
		panic(err)
	}

	f, err := multipart.NewReader(bytes.NewReader(rawData), boundary).ReadForm(32 << 20)
	if err != nil {
		panic(err)
	}
	const (
		formFieldProfile          = "profile"
		formFieldPreviousProfile  = "prev_profile"
		formFieldSampleTypeConfig = "sample_type_config"
	)

	Profile, err := form.ReadField(f, formFieldProfile)
	if err != nil {
		panic(err)
	}
	PreviousProfile, err := form.ReadField(f, formFieldPreviousProfile)
	if err != nil {
		panic(err)
	}

	stBS, err := form.ReadField(f, formFieldSampleTypeConfig)
	if err != nil {
		panic(err)
	}
	var config map[string]*tree.SampleTypeConfig
	if stBS != nil {
		if err = json.Unmarshal(stBS, &config); err != nil {
			panic(err)
		}
	} else {
		config = tree.DefaultSampleTypeMapping
	}
	_ = Profile
	_ = PreviousProfile

	decompres := func(b []byte) []byte {
		if len(b) < 2 {
			return b
		}
		if b[0] == 0x1f && b[1] == 0x8b {
			gzipr, err := gzip.NewReader(bytes.NewReader(b))
			if err != nil {
				panic(err)
			}
			defer gzipr.Close()
			var buf bytes.Buffer

			//defer bytebufferpool.Put(buf)

			if _, err = io.Copy(&buf, gzipr); err != nil {
				panic(err)
			}
			return buf.Bytes()
		}
		return b
	}
	if true {
		Profile = decompres(Profile)
		PreviousProfile = decompres(PreviousProfile)
	}
	//fmt.Println(config)
	elem := &testcase{Profile, PreviousProfile, config}
	return elem
}

//todo test protobufs from our go,ruby,dotnet integrations

func BenchmarkUnmarshal(b *testing.B) {
	now := time.Now()
	testcases := readCorpus("/home/korniltsev/Downloads/pprofs_short")
	for i := 0; i < b.N; i++ {
		for _, t := range testcases {

			parser := pprof.NewParser(
				pprof.ParserConfig{
					SampleTypes: t.config,
					Putter:      putter,
				})

			parser.ParsePprof(context.TODO(), now, now, t.profile)
		}

	}
}

func BenchmarkStreaming(b *testing.B) {
	now := time.Now()
	testcases := readCorpus("/home/korniltsev/Downloads/pprofs_short")

	for i := 0; i < b.N; i++ {
		for _, t := range testcases {

			config := t.config
			profile := t.profile

			parser := streaming.NewStreamingParser(
				streaming.ParserConfig{
					SampleTypes: config,
					Putter:      putter,
				})

			parser.ParsePprof(context.TODO(), now, now, profile)
		}

	}
}

func BenchmarkUnmarshalBig(b *testing.B) {
	now := time.Now()
	testcases := readCorpus("/home/korniltsev/Downloads/pprofs")
	for i := 0; i < b.N; i++ {
		for _, t := range testcases {

			parser := pprof.NewParser(
				pprof.ParserConfig{
					SampleTypes: t.config,
					Putter:      putter,
				})

			parser.ParsePprof(context.TODO(), now, now, t.profile)
		}

	}
}

func BenchmarkStreamingBig(b *testing.B) {
	now := time.Now()
	testcases := readCorpus("/home/korniltsev/Downloads/pprofs")

	for i := 0; i < b.N; i++ {
		for _, t := range testcases {

			config := t.config
			profile := t.profile

			parser := streaming.NewStreamingParser(
				streaming.ParserConfig{
					SampleTypes: config,
					Putter:      putter,
				})

			parser.ParsePprof(context.TODO(), now, now, profile)
		}

	}
}

var smallDir = "/home/korniltsev/Downloads/pprofs_short"
var smallItemIndex = 0
var bigDir = "/home/korniltsev/Downloads/pprofs"
var bigItemIndex = 0

func BenchmarkSingleSmallMolecule(b *testing.B) {
	t := readCorpusItemI(smallDir, smallItemIndex)
	now := time.Now()
	for i := 0; i < b.N; i++ {
		config := t.config
		profile := t.profile
		parser := streaming.NewStreamingParser(streaming.ParserConfig{SampleTypes: config, Putter: putter})
		parser.ParsePprof(context.TODO(), now, now, profile)
	}
}

func BenchmarkSingleSmallUnmarshal(b *testing.B) {
	t := readCorpusItemI(smallDir, smallItemIndex)
	now := time.Now()
	for i := 0; i < b.N; i++ {
		config := t.config
		profile := t.profile
		parser := pprof.NewParser(pprof.ParserConfig{SampleTypes: config, Putter: putter})
		parser.ParsePprof(context.TODO(), now, now, profile)
	}
}

func BenchmarkSingleBigMolecule(b *testing.B) {
	t := readCorpusItemI(bigDir, bigItemIndex)
	now := time.Now()
	for i := 0; i < b.N; i++ {
		config := t.config
		profile := t.profile
		parser := streaming.NewStreamingParser(streaming.ParserConfig{SampleTypes: config, Putter: putter})
		parser.ParsePprof(context.TODO(), now, now, profile)
	}
}

func BenchmarkSingleBigUnmarshal(b *testing.B) {
	t := readCorpusItemI(bigDir, bigItemIndex)
	now := time.Now()
	for i := 0; i < b.N; i++ {
		config := t.config
		profile := t.profile
		parser := pprof.NewParser(pprof.ParserConfig{SampleTypes: config, Putter: putter})
		parser.ParsePprof(context.TODO(), now, now, profile)
	}
}