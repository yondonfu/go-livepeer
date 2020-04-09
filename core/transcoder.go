package core

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/livepeer/go-livepeer/common"
	"github.com/livepeer/go-livepeer/monitor"
	"github.com/livepeer/lpms/ffmpeg"

	"github.com/golang/glog"
)

type Transcoder interface {
	Transcode(job string, fname string, profiles []ffmpeg.VideoProfile) (*TranscodeData, error)
}

type LocalTranscoder struct {
	workDir string
}

var WorkDir string

func (lt *LocalTranscoder) Transcode(job string, fname string, profiles []ffmpeg.VideoProfile) (*TranscodeData, error) {
	// Set up in / out config
	in := &ffmpeg.TranscodeOptionsIn{
		Fname: fname,
		Accel: ffmpeg.Software,
	}
	opts := profilesToTranscodeOptions(lt.workDir, ffmpeg.Software, profiles)

	_, seqNo, parseErr := parseURI(fname)
	start := time.Now()

	res, err := ffmpeg.Transcode3(in, opts)
	if err != nil {
		return nil, err
	}

	if monitor.Enabled && parseErr == nil {
		// This will run only when fname is actual URL and contains seqNo in it.
		// When orchestrator works as transcoder, `fname` will be relative path to file in local
		// filesystem and will not contain seqNo in it. For that case `SegmentTranscoded` will
		// be called in orchestrator.go
		monitor.SegmentTranscoded(0, seqNo, time.Since(start), common.ProfilesNames(profiles))
	}

	return resToTranscodeData(res, opts)
}

func NewLocalTranscoder(workDir string) Transcoder {
	return &LocalTranscoder{workDir: workDir}
}

type NvidiaTranscoder struct {
	device  string
	session *ffmpeg.Transcoder
}

func (nv *NvidiaTranscoder) Transcode(job string, fname string, profiles []ffmpeg.VideoProfile) (*TranscodeData, error) {

	in := &ffmpeg.TranscodeOptionsIn{
		Fname:  fname,
		Accel:  ffmpeg.Nvidia,
		Device: nv.device,
	}
	out := profilesToTranscodeOptions(WorkDir, ffmpeg.Nvidia, profiles)
	res, err := nv.session.Transcode(in, out)
	if err != nil {
		return nil, err
	}
	return resToTranscodeData(res, out)
}

func NewNvidiaTranscoder(gpu string) TranscoderSession {
	return &NvidiaTranscoder{
		device:  gpu,
		session: ffmpeg.NewTranscoder(),
	}
}

func (nv *NvidiaTranscoder) Stop() {
	nv.session.StopTranscoder()
}

func parseURI(uri string) (string, uint64, error) {
	var mid string
	var seqNo uint64
	parts := strings.Split(uri, "/")
	if len(parts) < 3 {
		return mid, seqNo, fmt.Errorf("BadURI")
	}
	mid = parts[len(parts)-2]
	parts = strings.Split(parts[len(parts)-1], ".")
	seqNo, err := strconv.ParseUint(parts[0], 10, 64)
	return mid, seqNo, err
}

func resToTranscodeData(res *ffmpeg.TranscodeResults, opts []ffmpeg.TranscodeOptions) (*TranscodeData, error) {
	if len(res.Encoded) != len(opts) {
		return nil, errors.New("lengths of results and options different")
	}

	// Convert results into in-memory bytes following the expected API
	segments := make([]*TranscodedSegmentData, len(opts), len(opts))
	for i := range opts {
		oname := opts[i].Oname
		o, err := ioutil.ReadFile(oname)
		if err != nil {
			glog.Error("Cannot read transcoded output for ", oname)
			return nil, err
		}
		segments[i] = &TranscodedSegmentData{Data: o, Pixels: res.Encoded[i].Pixels}
		os.Remove(oname)
	}

	return &TranscodeData{
		Segments: segments,
		Pixels:   res.Decoded.Pixels,
	}, nil
}

func profilesToTranscodeOptions(workDir string, accel ffmpeg.Acceleration, profiles []ffmpeg.VideoProfile) []ffmpeg.TranscodeOptions {
	opts := make([]ffmpeg.TranscodeOptions, len(profiles), len(profiles))
	for i := range profiles {
		o := ffmpeg.TranscodeOptions{
			Oname:        fmt.Sprintf("%s/out_%s.tempfile", workDir, common.RandName()),
			Profile:      profiles[i],
			Accel:        accel,
			AudioEncoder: ffmpeg.ComponentOptions{Name: "copy"},
		}
		opts[i] = o
	}
	return opts
}
