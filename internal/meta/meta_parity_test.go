package meta

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 交叉比对：同一批 $I 字节，Python 的 rb_meta.parse_i_file 与 Go 的 parseIFile 结果必须一致。
// fixture 由 testdata/regen_py_meta.py 生成，含 deleted_at 的时区口径、epoch 的浮点写法、
// display 的取名规则，以及一批「Python 抛 OSError → 归为元数据损坏」的极端 FILETIME。

type metaCase struct {
	Stem          string  `json:"stem"`
	MetaVersion   int64   `json:"meta_version"`
	Size          int64   `json:"size"`
	DeletedAt     string  `json:"deleted_at"`
	Epoch         float64 `json:"epoch"`
	OrigPath      string  `json:"orig_path"`
	Display       string  `json:"display"`
	Unrecoverable bool    `json:"unrecoverable"`
}

type pyMetaFixture struct {
	LocalUTCOffsetNow int        `json:"local_utc_offset_now"`
	Cases             []metaCase `json:"cases"`
	ErrorCases        []struct {
		Stem    string `json:"stem"`
		PyError string `json:"py_error"`
	} `json:"error_cases"`
}

func loadPyMeta(t *testing.T) *pyMetaFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "py_meta.json"))
	if err != nil {
		t.Fatalf("读 fixture 失败（先跑 testdata/regen_py_meta.py）: %v", err)
	}
	var fx pyMetaFixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatalf("fixture 解析失败: %v", err)
	}
	if len(fx.Cases) == 0 || len(fx.ErrorCases) == 0 {
		t.Fatalf("fixture 不完整: %d 例正常 / %d 例必错", len(fx.Cases), len(fx.ErrorCases))
	}
	return &fx
}

func TestMatchesPythonMeta(t *testing.T) {
	fx := loadPyMeta(t)
	_, offset := time.Now().Zone()
	if offset != fx.LocalUTCOffsetNow {
		t.Skipf("生成 fixture 时的时区偏移 %+d 秒，本机现在 %+d 秒，跳过时区相关比对",
			fx.LocalUTCOffsetNow, offset)
	}
	for _, c := range fx.Cases {
		t.Run(c.Stem, func(t *testing.T) {
			iPath := filepath.Join("testdata", "isample", "$I"+c.Stem)
			head, err := parseIFile(iPath)
			if err != nil {
				t.Fatalf("Go 解析失败，Python 侧是好的: %v", err)
			}
			it := head.buildItem(iPath)
			if it.MetaVersion != c.MetaVersion {
				t.Errorf("meta_version = %d，Python %d", it.MetaVersion, c.MetaVersion)
			}
			if it.Size != c.Size {
				t.Errorf("size = %d，Python %d", it.Size, c.Size)
			}
			if it.DeletedAt != c.DeletedAt {
				t.Errorf("deleted_at = %q，Python %q", it.DeletedAt, c.DeletedAt)
			}
			if math.Abs(it.Epoch-c.Epoch) > 1e-3 {
				t.Errorf("epoch = %v，Python %v", it.Epoch, c.Epoch)
			}
			if it.OrigPath != c.OrigPath {
				t.Errorf("orig_path = %q，Python %q", it.OrigPath, c.OrigPath)
			}
			if it.Display != c.Display {
				t.Errorf("display = %q，Python %q", it.Display, c.Display)
			}
			if it.Unrecoverable != c.Unrecoverable {
				t.Errorf("unrecoverable = %v，Python %v", it.Unrecoverable, c.Unrecoverable)
			}
		})
	}
}

// Python 侧渲染不了（Windows localtime 抛 OSError）的 FILETIME，在两边都得算「元数据损坏」。
func TestMatchesPythonMetaErrors(t *testing.T) {
	fx := loadPyMeta(t)
	for _, c := range fx.ErrorCases {
		t.Run(c.Stem, func(t *testing.T) {
			if c.PyError != "OSError" {
				t.Errorf("fixture 记录的异常类型变了: %s", c.PyError)
			}
			iPath := filepath.Join("testdata", "isample", "$I"+c.Stem)
			if head, err := parseIFile(iPath); err == nil {
				t.Errorf("Go 竟然解析成功了（%+v），Python 侧这里是 OSError → 应当归为元数据损坏", head)
			}
		})
	}
}

// 走完整 Scan：这类条目要落进「元数据损坏」，epoch=0、display 用 $I 文件名尾巴，
// 并且留一条异常记录——和 Python 的 rb_meta.scan 同一口径。
func TestScanClassifiesUnrepresentableFiletime(t *testing.T) {
	bin := t.TempDir()
	for _, stem := range []string{"200001ft0", "100001winter"} {
		raw, err := os.ReadFile(filepath.Join("testdata", "isample", "$I"+stem))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bin, "$I"+stem), raw, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bin, "$R"+stem), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	items, issues, _ := Scan([]BinDir{{Drive: "Z", Path: bin}}, 0)
	if len(items) != 2 {
		t.Fatalf("条目数 = %d，期望 2", len(items))
	}
	byName := map[string]*Item{}
	for _, it := range items {
		byName[filepath.Base(it.IPath)] = it
	}
	bad := byName["$I200001ft0"]
	if bad == nil {
		t.Fatal("坏条目没被收进来")
	}
	if bad.OrigPath != "(元数据损坏)" || bad.Epoch != 0 || bad.DeletedAt != "1970-01-01 00:00:00" {
		t.Errorf("坏条目没按元数据损坏处理: %+v", bad)
	}
	if bad.Display != "200001ft0" || bad.MetaVersion != -1 || bad.Size != 0 {
		t.Errorf("坏条目的 display/版本/大小不符: %+v", bad)
	}
	if !bad.Exists {
		t.Error("$R 在时应标 exists")
	}
	if len(issues) != 1 || !strings.Contains(issues[0], "解析失败") {
		t.Errorf("异常记录 = %v，期望一条「解析失败」", issues)
	}
	good := byName["$I100001winter"]
	if good == nil || good.OrigPath != `Z:\work\build1\out\chunk-46.js` || good.Display != "chunk-46.js" {
		t.Errorf("好条目被带坏了: %+v", good)
	}
}
