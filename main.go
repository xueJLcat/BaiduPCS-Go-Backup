package main

import (
	"fmt"
	"github.com/iikira/BaiduPCS-Go/internal/pcscommand"
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
	Version  = "3.6.7"
	reloadFn = func(c *cli.Context) error {
		err := pcsconfig.Config.Reload()
		if err != nil {
			fmt.Printf("重载配置错误: %s\n", err)
		}
		return nil
	}
	saveFunc = func(c *cli.Context) error {
		err := pcsconfig.Config.Save()
		if err != nil {
			fmt.Printf("保存配置错误: %s\n", err)
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

	if pcsweb.GlobalSessions == nil {
		pcsweb.GlobalSessions, err = pcsweb.NewSessionManager("memory", "goSessionid", 90 * 24 * 3600)
		if err != nil {
			fmt.Println(err)
			return
		}
		pcsweb.GlobalSessions.Init()
		go pcsweb.GlobalSessions.GC()
	}
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

		if err := pcsweb.StartServer(5299, true); err != nil {
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
				fmt.Println(pcsweb.StartServer(c.Uint("port"), c.Bool("access")))
				return nil
			},
			Flags: []cli.Flag{
				cli.UintFlag{
					Name:  "port",
					Usage: "自定义端口",
					Value: 5299,
				},
				cli.BoolFlag{
					Name:  "access",
					Usage: "是否允许外网访问",
					Hidden: false,
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
		{
			Name:        "logout",
			Usage:       "退出百度帐号",
			Description: "退出当前登录的百度帐号",
			Category:    "百度帐号",
			Before:      reloadFn,
			After:       saveFunc,
			Action: func(c *cli.Context) error {
				if pcsconfig.Config.NumLogins() == 0 {
					fmt.Println("未设置任何百度帐号, 不能退出")
					return nil
				}

				var (
					confirm    string
					activeUser = pcsconfig.Config.ActiveUser()
				)

				if !c.Bool("y") {
					fmt.Printf("确认退出百度帐号: %s ? (y/n) > ", activeUser.Name)
					_, err := fmt.Scanln(&confirm)
					if err != nil || (confirm != "y" && confirm != "Y") {
						return err
					}
				}

				deletedUser, err := pcsconfig.Config.DeleteUser(&pcsconfig.BaiduBase{
					UID: activeUser.UID,
				})
				if err != nil {
					fmt.Printf("退出用户 %s, 失败, 错误: %s\n", activeUser.Name, err)
				}

				fmt.Printf("退出用户成功, %s\n", deletedUser.Name)
				return nil
			},
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "y",
					Usage: "确认退出帐号",
				},
			},
		},

		{
			Name:        "loglist",
			Usage:       "列出帐号列表",
			Description: "列出所有已登录的百度帐号",
			Category:    "百度帐号",
			Before:      reloadFn,
			Action: func(c *cli.Context) error {
				list := pcsconfig.Config.BaiduUserList()
				fmt.Println(list.String())
				return nil
			},
		},
		{
			Name:        "who",
			Usage:       "获取当前帐号",
			Description: "获取当前帐号的信息",
			Category:    "百度帐号",
			Before:      reloadFn,
			Action: func(c *cli.Context) error {
				activeUser := pcsconfig.Config.ActiveUser()
				fmt.Printf("当前帐号 uid: %d, 用户名: %s, 性别: %s, 年龄: %.1f\n", activeUser.UID, activeUser.Name, activeUser.Sex, activeUser.Age)
				return nil
			},
		},
		{
			Name:        "quota",
			Usage:       "获取网盘配额",
			Description: "获取网盘的总储存空间, 和已使用的储存空间",
			Category:    "百度网盘",
			Before:      reloadFn,
			Action: func(c *cli.Context) error {
				pcscommand.RunGetQuota()
				return nil
			},
		},
		{
			Name:        "config",
			Usage:       "显示和修改程序配置项",
			Description: "显示和修改程序配置项",
			Category:    "配置",
			Before:      reloadFn,
			After:       saveFunc,
			Action: func(c *cli.Context) error {
				fmt.Printf("----\n运行 %s config set 可进行设置配置\n\n当前配置:\n", app.Name)
				pcsconfig.Config.PrintTable()
				return nil
			},
			Subcommands: []cli.Command{
				{
					Name:      "set",
					Usage:     "修改程序配置项",
					UsageText: app.Name + " config set [arguments...]",
					Description: `
	例子:
		BaiduPCS-Go config set -appid=260149
		BaiduPCS-Go config set -enable_https=false
		BaiduPCS-Go config set -user_agent="netdisk;1.0"
		BaiduPCS-Go config set -cache_size 16384 -max_parallel 200 -savedir D:/download`,
					Action: func(c *cli.Context) error {
						if c.NumFlags() <= 0 || c.NArg() > 0 {
							cli.ShowCommandHelp(c, c.Command.Name)
							return nil
						}

						if c.IsSet("appid") {
							pcsconfig.Config.SetAppID(c.Int("appid"))
						}
						if c.IsSet("enable_https") {
							pcsconfig.Config.SetEnableHTTPS(c.Bool("enable_https"))
						}
						if c.IsSet("user_agent") {
							pcsconfig.Config.SetUserAgent(c.String("user_agent"))
						}
						if c.IsSet("cache_size") {
							pcsconfig.Config.SetCacheSize(c.Int("cache_size"))
						}
						if c.IsSet("max_parallel") {
							pcsconfig.Config.SetMaxParallel(c.Int("max_parallel"))
						}
						if c.IsSet("max_upload_parallel") {
							pcsconfig.Config.SetMaxUploadParallel(c.Int("max_upload_parallel"))
						}
						if c.IsSet("max_download_load") {
							pcsconfig.Config.SetMaxDownloadLoad(c.Int("max_download_load"))
						}
						if c.IsSet("savedir") {
							pcsconfig.Config.SetSaveDir(c.String("savedir"))
						}
						if c.IsSet("proxy") {
							pcsconfig.Config.SetProxy(c.String("proxy"))
						}
						if c.IsSet("local_addrs") {
							pcsconfig.Config.SetLocalAddrs(c.String("local_addrs"))
						}

						err := pcsconfig.Config.Save()
						if err != nil {
							fmt.Println(err)
							return err
						}

						pcsconfig.Config.PrintTable()
						fmt.Printf("\n保存配置成功!\n\n")

						return nil
					},
					Flags: []cli.Flag{
						cli.IntFlag{
							Name:  "appid",
							Usage: "百度 PCS 应用ID",
						},
						cli.IntFlag{
							Name:  "cache_size",
							Usage: "下载缓存",
						},
						cli.IntFlag{
							Name:  "max_parallel",
							Usage: "下载网络连接的最大并发量",
						},
						cli.IntFlag{
							Name:  "max_upload_parallel",
							Usage: "上传网络连接的最大并发量",
						},
						cli.IntFlag{
							Name:  "max_download_load",
							Usage: "同时进行下载文件的最大数量",
						},
						cli.StringFlag{
							Name:  "savedir",
							Usage: "下载文件的储存目录",
						},
						cli.BoolFlag{
							Name:  "enable_https",
							Usage: "启用 https",
						},
						cli.StringFlag{
							Name:  "user_agent",
							Usage: "浏览器标识",
						},
						cli.StringFlag{
							Name:  "proxy",
							Usage: "设置代理, 支持 http/socks5 代理",
						},
						cli.StringFlag{
							Name:  "local_addrs",
							Usage: "设置本地网卡地址, 多个地址用逗号隔开",
						},
					},
				},
			},
		},
	}

	app.Run(os.Args)
}
