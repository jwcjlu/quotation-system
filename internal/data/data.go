package data

import (
	"caichip/internal/conf"
	"caichip/pkg/platform/ickey"
	"caichip/pkg/platform/szlcsc"
)

// Data .
type Data struct {
}

// NewData .
func NewData(c *conf.Bootstrap) (*Data, error) {
	return &Data{}, nil
}

// NewSearchers 创建平台搜索客户端列表
func NewSearchers(c *conf.Bootstrap) []interface{} {
	var list []interface{}
	if c.Platform != nil {
		if c.Platform.Ickey != nil {
			crawlerPath := c.Platform.Ickey.CrawlerPath
			crawlerScript := c.Platform.Ickey.CrawlerScript
			if crawlerPath == "" {
				crawlerPath = "python"
			}
			if crawlerScript == "" {
				crawlerScript = "ickey_crawler.py"
			}
			list = append(list, ickey.NewClient(
				c.Platform.Ickey.SearchURL,
				c.Platform.Ickey.Timeout,
				crawlerPath,
				crawlerScript,
			))
		}
		if c.Platform.Szlcsc != nil {
			list = append(list, szlcsc.NewClient(
				c.Platform.Szlcsc.SearchURL,
				c.Platform.Szlcsc.Timeout,
			))
		}
	}
	if len(list) == 0 {
		list = append(list, ickey.NewClient("https://search.ickey.cn/", 15, "python", "ickey_crawler.py"))
		list = append(list, szlcsc.NewClient("https://www.szlcsc.com/", 15))
	}
	return list
}
