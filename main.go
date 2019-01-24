package main

import (
	"fmt"
	"github.com/iikira/BaiduPCS-Go/internal/pcsconfig"
	_ "github.com/iikira/BaiduPCS-Go/internal/pcsinit"
	"github.com/iikira/BaiduPCS-Go/internal/pcsweb"
	"github.com/iikira/BaiduPCS-Go/pcsutil"
	"github.com/iikira/BaiduPCS-Go/pcsverbose"
	"github.com/iikira/BaiduPCS-Go/requester"
	"github.com/urfave/cli"
	"os"
	"os/exec"
	"runtime"
)

var (
	Version  = "3.6.3"
	reloadFn = func(c *cli.Context) error {
		err := pcsconfig.Config.Reload()
		if err != nil {
			fmt.Printf("重载配置错误: %s\n", err)
		}
		return nil
	}
)

func init() {
	pcsutil.ChWorkDir()

	err := pcsconfig.Config.Init()
	switch err {
	case nil:
	case pcsconfig.ErrConfigFileNoPermission, pcsconfig.ErrConfigContentsParseError:
		fmt.Fprintf(os.Stderr, "FATAL ERROR: config file error: %s\n", err)
		os.Exit(1)
	default:
		fmt.Printf("WARNING: config init error: %s\n", err)
	}

	// 启动缓存回收
	requester.TCPAddrCache.GC()
}

func main() {
	defer pcsconfig.Config.Close()

	app := cli.NewApp()
	app.Name = "BaiduPCS-Go"
	app.Version = Version
	liuzhuoling := cli.Author{
		Name:  "liuzhuoling",
		Email: "liuzhuoling2011@hotmail.com",
	}
	iikira := cli.Author{
		Name:  "iikira",
		Email: "i@mail.iikira.com",
	}
	app.Authors = []cli.Author{liuzhuoling, iikira}
	app.Description = "这个软件可以让你高效的使用百度云"
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:        "verbose",
			Usage:       "启用调试",
			EnvVar:      pcsverbose.EnvVerbose,
			Destination: &pcsverbose.IsVerbose,
		},
	}
	app.Action = func(c *cli.Context) {
		fmt.Printf("打开浏览器, 输入 http://localhost:5299 查看效果\n")
		//对于Windows和Mac，调用系统默认浏览器打开 http://localhost:5299
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.Command("CMD", "/C", "start", "http://localhost:5299")
			if err := cmd.Start(); err != nil {
				fmt.Println(err.Error())
			}
		} else if runtime.GOOS == "darwin" {
			cmd = exec.Command("open", "http://localhost:5299")
			if err := cmd.Start(); err != nil {
				fmt.Println(err.Error())
			}
		}

		if err := pcsweb.StartServer(5299); err != nil {
			fmt.Println(err.Error())
		}
	}
	app.Commands = []cli.Command{
		{
			Name:     "web",
			Usage:    "启用 web 客户端",
			Category: "其他",
			Before:   reloadFn,
			Action: func(c *cli.Context) error {
				fmt.Printf("打开浏览器, 输入: http://localhost:%d 查看效果\n", c.Uint("port"))
				fmt.Println(pcsweb.StartServer(c.Uint("port")))
				return nil
			},
			Flags: []cli.Flag{
				cli.UintFlag{
					Name:  "port",
					Usage: "自定义端口",
					Value: 5299,
				},
			},
		},
		{
			Name:     "env",
			Usage:    "显示程序环境变量",
			Category: "其他",
			Action: func(c *cli.Context) error {
				envStr := "%s=\"%s\"\n"
				envVar, ok := os.LookupEnv(pcsverbose.EnvVerbose)
				if ok {
					fmt.Printf(envStr, pcsverbose.EnvVerbose, envVar)
				} else {
					fmt.Printf(envStr, pcsverbose.EnvVerbose, "0")
				}

				envVar, ok = os.LookupEnv(pcsconfig.EnvConfigDir)
				if ok {
					fmt.Printf(envStr, pcsconfig.EnvConfigDir, envVar)
				} else {
					fmt.Printf(envStr, pcsconfig.EnvConfigDir, pcsconfig.GetConfigDir())
				}

				return nil
			},
		},
	}

	app.Run(os.Args)
}
