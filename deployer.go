package main

import (
	"bytes"
	"fmt"
	"github.com/GeertJohan/go.rice"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	systemFolder   = "/etc/systemd/system/"
	dockerService  = "docker.service"
	flannelService = "flannel.service"
	flannelPath    = "/opt/bin/flanneld"
	flannelCDN     = "https://24827.selcdn.ru/cydev/flanneld"
	envFile        = "/run/flannel/subnet.env"
	poolTime       = time.Millisecond * 200
	maxTimeout     = 60 * time.Second
)

var (
	templates *rice.Box
)

func downloadFlannel() {
	if exec.Command("systemctl", "status", "flannel").Run() == nil {
		fmt.Println("flannel service already exists")
		return
	}
	fmt.Printf("downloading flannel...")
	if err := os.MkdirAll(filepath.Dir(flannelPath), 0777); err != nil {
		log.Fatal(err)
	}
	dst, err := os.Create(flannelPath)
	check(err)
	defer dst.Close()
	req, _ := http.NewRequest("GET", flannelCDN, nil)
	res, err := http.DefaultClient.Do(req)
	check(err)
	if res.StatusCode != http.StatusOK {
		log.Fatalf("Bad status %s", res.Status)
	}
	src := res.Body
	defer src.Close()
	if _, err := io.Copy(dst, src); err != nil {
		log.Fatal(err)
	}
	must(fmt.Sprintf("chmod +x %s", flannelPath))
	fmt.Printf("ok\n")
}

func must(args string) {
	cmd := exec.Command("/bin/bash", "-c", args)
	cmd.Stderr = os.Stderr
	check(cmd.Run())
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func stopDocker() {
	fmt.Print("stoping docker...")
	must("systemctl stop docker")
	fmt.Print("ok\n")
}

func configureDockerNetwork() {
	buffer := new(bytes.Buffer)
	cmd := exec.Command("ifconfig")
	cmd.Stdout = buffer
	check(cmd.Run())
	if strings.Contains(buffer.String(), "docker0") {
		fmt.Print("removing docker0...")
		must("ip link set dev docker0 down")
		must("brctl delbr docker0")
		fmt.Print("ok\n")
	} else {
		fmt.Println("docker0 already removed")
	}

	fmt.Print("restarting docker...")
	must("systemctl restart docker")
	must("docker ps")
	fmt.Print("ok\n")
}

func createDockerService() {
	fmt.Printf("creating docker service file...")
	dst, err := os.Create(filepath.Join(systemFolder, dockerService))
	check(err)
	defer dst.Close()
	src, err := templates.Open(dockerService)
	_, err = io.Copy(dst, src)
	check(err)
	must("systemctl daemon-reload")
	fmt.Printf("ok\n")
}

func createFlannelService() {
	if exec.Command("systemctl", "status", "flannel").Run() == nil {
		fmt.Println("flannel service already exists")
		return
	}
	fmt.Print("creating docker service file...")
	dst, err := os.Create(filepath.Join(systemFolder, flannelService))
	check(err)
	defer dst.Close()
	src, err := templates.Open(flannelService)
	_, err = io.Copy(dst, src)
	check(err)
	must("systemctl daemon-reload")
	fmt.Printf("ok\n")
}

func waitForFile() {
	fmt.Print("waiting for flannel subnet file...")
	start := time.Now()
	for {
		_, err := os.Stat(envFile)
		if err == nil {
			fmt.Print("ok\n")
			return
		}
		time.Sleep(poolTime)
		if time.Now().Sub(start) > maxTimeout {
			fmt.Print("timed out\n")
			log.Fatalln("Waiting for flannel subnet file timed out")
		}
	}
}

func startFlannel() {
	fmt.Print("starting flannel...")
	must("systemctl start flannel")
	fmt.Printf("ok\n")
}

func main() {
	templates = rice.MustFindBox("templates")
	fmt.Println("deploying flannel on coreos")
	stopDocker()
	createDockerService()
	downloadFlannel()
	createFlannelService()
	startFlannel()
	waitForFile()
	configureDockerNetwork()
	fmt.Println("operations complete")
}
