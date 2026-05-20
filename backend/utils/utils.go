package utils

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var runEnv string = "local"

func init() {

	//环境变量 > .env 文件
	env, _ := GetEnvFromFile(ENV_FILE, "APP_ENV")
	if os.Getenv("APP_ENV") != "" {
		env = os.Getenv("APP_ENV")
	}

	env = strings.TrimRight(env, "\r\n")
	env = strings.TrimSpace(env)
	env = strings.ToLower(env)
	if env != "prod" && env != "stag" && env != "uat" && env != "dev" && env != "test" && env != "local" {
		env = "local"
	}
	runEnv = env
}

func CalcPageOffset(page string, pageSize string) (int64, int64) {
	n, _ := strconv.Atoi(page)
	if n == 0 {
		n = 1
	}

	size, _ := strconv.Atoi(pageSize)
	switch {
	case size > 100:
		size = 100
	case size <= 0:
		size = 20
	}

	offset := (n - 1) * size

	return int64(offset), int64(size)
}

func CheckFileExist(filename string) bool {
	_, err := os.Open(filename)
	return err == nil
}

func GetLocalIp() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}

	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet == nil || ipNet.IP.IsLoopback() {
			continue
		}

		if ipNet.IP.To4() != nil { // IPv4
			return ipNet.IP.String()
		} else if len(ipNet.IP) > 15 && ipNet.IP[0] == 239 && ipNet.IP[1] == 255 { // IPv6 link-local unicast
			return fmt.Sprintf("fe80::%x", ipNet.IP[2:])
		}
	}

	return ""
}

func GetList(value string, sep string) []string {
	var res = []string{}

	myList := strings.Split(value, sep)
	for _, v := range myList {
		if v == "" {
			continue
		}
		res = append(res, v)
	}

	return res
}

func StrCmp(str1 string, str2 string) bool {
	if strings.EqualFold(str1, str2) {
		return true
	} else {
		return false
	}
}

func AppendResetLog() {
	f, err := os.OpenFile(RESET_LOGFILE, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0660)
	if err == nil {
		now := time.Now()
		log := fmt.Sprintf("%s %s restart at %s\n", APP_NAME, VERSION, now.Format("20060102150405"))
		f.WriteString(log)
		f.Close()
	}
}

func LogPid() {
	f, err := os.OpenFile(PID_LOGFILE, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0660)
	if err == nil {
		pid := syscall.Getpid()
		f.WriteString(fmt.Sprintf("%d", pid))
		f.Close()
	}
}

func GetPid() int {
	data, err := os.ReadFile(PID_LOGFILE)
	if err != nil {
		fmt.Println(PID_LOGFILE, "not found!")
		return 0
	}

	pid, _ := strconv.Atoi(string(data))
	return pid
}

func GetConfigFile() (string, string) {
	env := strings.TrimSpace(runEnv)
	if env == "" {
		env = "local"
	}

	return fmt.Sprintf(CONF_FILE, env), env
}

func GetRunEnv() string {
	return runEnv
}

func GetFrontendEntry(env string) string {
	if len(runEnv) == 0 {
		return "../admin-web"
	} else {
		return "../admin-web." + env
	}
}

func ExtractStreamPath(rawUrl string) string {
	u, err := url.Parse(rawUrl)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(u.Path, "/")
}

// GetEnvFromFile 读取 .env 文件并返回指定 key 的值
func GetEnvFromFile(filename, key string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 跳过空行或注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, key+"=") {
			value := strings.TrimPrefix(line, key+"=")
			return value, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", errors.New("key not found in file")
}

func GetContainerMemoryLimit() (uint64, error) {
	// cgroup v2
	if data, err := os.ReadFile("/sys/fs/cgroup/memory.max"); err == nil {
		s := strings.TrimSpace(string(data))
		if s != "max" {
			if v, err := strconv.ParseUint(s, 10, 64); err == nil {
				return v, nil
			}
		}
	}

	// cgroup v1
	if data, err := os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes"); err == nil {
		if v, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64); err == nil {
			// v1 unlimited 会返回一个非常大的数
			if v < 1<<60 {
				return v, nil
			}
		}
	}

	return 0, errors.New("cannot detect container memory limit")
}
