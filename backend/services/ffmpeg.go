package services

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ffprobe 输出结构体
type FFProbeOutput2 struct {
	Streams []struct {
		CodecType    string `json:"codec_type"`
		CodecName    string `json:"codec_name"`
		Profile      string `json:"profile,omitempty"`
		Width        int    `json:"width,omitempty"`
		Height       int    `json:"height,omitempty"`
		BitRate      string `json:"bit_rate,omitempty"`
		AvgFrameRate string `json:"avg_frame_rate,omitempty"`
		SampleRate   string `json:"sample_rate,omitempty"`
		Channels     int    `json:"channels,omitempty"`
	} `json:"streams"`
}

// 结果结构体
type StreamInfo2 struct {
	// 视频
	VideoCodec   string
	VideoProfile string
	Resolution   string
	FPS          string
	VideoBitrate int

	// 音频
	AudioCodec    string
	AudioSampleHz string
	AudioChannels string
	AudioBitrate  int
}

// ProbeStream 使用 ffprobe 解析 RTMP 流
func ProbeStream(url string) (*StreamInfo2, error) {
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		url,
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe error: %w", err)
	}

	var ffout FFProbeOutput2
	if err := json.Unmarshal(output, &ffout); err != nil {
		return nil, fmt.Errorf("json parse error: %w", err)
	}

	info := &StreamInfo2{}

	for _, s := range ffout.Streams {
		switch s.CodecType {
		case "video":
			info.VideoCodec = s.CodecName
			info.VideoProfile = s.Profile
			if s.Width > 0 && s.Height > 0 {
				info.Resolution = fmt.Sprintf("%dx%d", s.Width, s.Height)
			}
			if s.AvgFrameRate != "" && s.AvgFrameRate != "0/0" {
				info.FPS = parseFPS(s.AvgFrameRate)
			}
			info.VideoBitrate, _ = strconv.Atoi(s.BitRate)

		case "audio":
			info.AudioCodec = s.CodecName
			if s.SampleRate != "" {
				info.AudioSampleHz = s.SampleRate + " Hz"
			}
			if s.Channels > 0 {
				info.AudioChannels = strconv.Itoa(s.Channels)
			}
			info.AudioBitrate, _ = strconv.Atoi(s.BitRate)
		}
	}

	return info, nil
}

// 把 "30000/1001" 转换为 "29.97 fps"
func parseFPS(rate string) string {
	parts := strings.Split(rate, "/")
	if len(parts) == 2 {
		num, _ := strconv.ParseFloat(parts[0], 64)
		den, _ := strconv.ParseFloat(parts[1], 64)
		if den != 0 {
			return fmt.Sprintf("%.2f", num/den)
		}
	}
	return rate
}

func FfprobeStreamInfo2(inputStream string) (string, error) {
	info, err := ProbeStream(inputStream)
	if err != nil {
		fmt.Println(err)
		return "", fmt.Errorf("ffprobe error: %v", err)
	}

	parameter := fmt.Sprintf("Video:codec=%s,profile=%s,resolution=%s,fps=%s,bitrate=%d kbps ",
		info.VideoCodec, info.VideoProfile, info.Resolution, info.FPS, info.VideoBitrate/1e3)
	parameter += fmt.Sprintf("Audio:codec=%s,sample_rate=%s,channels=%s,bitrate=%d kbps",
		info.AudioCodec, info.AudioSampleHz, info.AudioChannels, info.AudioBitrate/1e3)
	return parameter, nil
}
