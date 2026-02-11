package aichat

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFileStorageDirect 直接测试 FileStorage 功能
func TestFileStorageDirect(t *testing.T) {
	// 创建临时目录
	tempDir := t.TempDir()
	storageDir := filepath.Join(tempDir, "storage")

	// 创建文件存储实例（不通过单例）
	fs := &FileStorage{
		dir:   storageDir,
		cache: make(map[int64]*GroupConfig),
	}

	// 确保目录存在
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}

	// 测试默认值
	gid := int64(123456)
	config := fs.Get(gid)

	if config.Rate != 0 {
		t.Errorf("默认触发概率应为0，得到: %d", config.Rate)
	}
	if config.Temp != 70 {
		t.Errorf("默认温度应为70，得到: %d", config.Temp)
	}
	if config.NoAgent != false {
		t.Errorf("默认NoAgent应为false，得到: %v", config.NoAgent)
	}
	if config.NoRecord != false {
		t.Errorf("默认NoRecord应为false，得到: %v", config.NoRecord)
	}
	if config.NoReplyAt != false {
		t.Errorf("默认NoReplyAt应为false，得到: %v", config.NoReplyAt)
	}
}

// TestFileStorageOperations 测试文件存储操作
func TestFileStorageOperations(t *testing.T) {
	tempDir := t.TempDir()
	storageDir := filepath.Join(tempDir, "storage")

	fs := &FileStorage{
		dir:   storageDir,
		cache: make(map[int64]*GroupConfig),
	}

	if err := os.MkdirAll(storageDir, 0755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}

	gid := int64(789012)

	// 测试保存和读取
	if err := fs.SaveRate(gid, 50); err != nil {
		t.Errorf("SaveRate失败: %v", err)
	}

	config := fs.Get(gid)
	if config.Rate != 50 {
		t.Errorf("期望Rate为50，得到: %d", config.Rate)
	}

	// 测试布尔值
	if err := fs.SaveNoRecord(gid, true); err != nil {
		t.Errorf("SaveNoRecord失败: %v", err)
	}

	config = fs.Get(gid)
	if !config.NoRecord {
		t.Errorf("期望NoRecord为true，得到: %v", config.NoRecord)
	}
}

// TestStorageMethods 测试 storage 结构体的方法
func TestStorageMethods(t *testing.T) {
	tempDir := t.TempDir()
	storageDir := filepath.Join(tempDir, "storage")

	fs := &FileStorage{
		dir:   storageDir,
		cache: make(map[int64]*GroupConfig),
	}

	if err := os.MkdirAll(storageDir, 0755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}

	gid := int64(345678)

	// 设置测试数据
	fs.SaveRate(gid, 30)
	fs.SaveTemp(gid, 80)
	fs.SaveNoRecord(gid, true)
	fs.SaveNoAgent(gid, false)
	fs.SaveNoReplyAt(gid, true)

	// 创建 storage 对象
	stor := storage{
		groupID: gid,
		fs:      fs,
	}

	// 测试方法
	if rate := stor.rate(); rate != 30 {
		t.Errorf("stor.rate() = %d, 期望 30", rate)
	}

	if temp := stor.temp(); temp != 0.80 {
		t.Errorf("stor.temp() = %f, 期望 0.80", temp)
	}

	if norecord := stor.norecord(); !norecord {
		t.Errorf("stor.norecord() = %v, 期望 true", norecord)
	}

	if noagent := stor.noagent(); noagent {
		t.Errorf("stor.noagent() = %v, 期望 false", noagent)
	}

	if noreplyat := stor.noreplyat(); !noreplyat {
		t.Errorf("stor.noreplyat() = %v, 期望 true", noreplyat)
	}
}

// TestStorageTemperature 测试温度转换
func TestStorageTemperature(t *testing.T) {
	tempDir := t.TempDir()
	storageDir := filepath.Join(tempDir, "storage")

	fs := &FileStorage{
		dir:   storageDir,
		cache: make(map[int64]*GroupConfig),
	}

	if err := os.MkdirAll(storageDir, 0755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}

	gid := int64(456789)

	// 测试各种温度值
	testCases := []struct {
		input    uint8
		expected float32
	}{
		{0, 0.70}, // 默认值
		{1, 0.01},
		{50, 0.50},
		{80, 0.80},
		{100, 1.00},
	}

	for _, tc := range testCases {
		fs.SaveTemp(gid, tc.input)

		stor := storage{
			groupID: gid,
			fs:      fs,
		}

		if temp := stor.temp(); temp != tc.expected {
			t.Errorf("温度 %d => 期望 %f, 得到 %f", tc.input, tc.expected, temp)
		}
	}
}

// TestMultipleGroups 测试多个群组
func TestMultipleGroups(t *testing.T) {
	tempDir := t.TempDir()
	storageDir := filepath.Join(tempDir, "storage")

	fs := &FileStorage{
		dir:   storageDir,
		cache: make(map[int64]*GroupConfig),
	}

	if err := os.MkdirAll(storageDir, 0755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}

	gid1 := int64(111111)
	gid2 := int64(222222)

	// 为不同群组设置不同配置
	fs.SaveRate(gid1, 20)
	fs.SaveTemp(gid1, 60)
	fs.SaveNoRecord(gid1, true)

	fs.SaveRate(gid2, 80)
	fs.SaveTemp(gid2, 90)
	fs.SaveNoRecord(gid2, false)

	// 验证配置隔离
	stor1 := storage{groupID: gid1, fs: fs}
	stor2 := storage{groupID: gid2, fs: fs}

	if stor1.rate() != 20 {
		t.Errorf("群组1触发概率应为20，得到: %d", stor1.rate())
	}
	if stor2.rate() != 80 {
		t.Errorf("群组2触发概率应为80，得到: %d", stor2.rate())
	}

	if stor1.temp() != 0.60 {
		t.Errorf("群组1温度应为0.60，得到: %f", stor1.temp())
	}
	if stor2.temp() != 0.90 {
		t.Errorf("群组2温度应为0.90，得到: %f", stor2.temp())
	}

	if !stor1.norecord() {
		t.Errorf("群组1norecord应为true，得到: %v", stor1.norecord())
	}
	if stor2.norecord() {
		t.Errorf("群组2norecord应为false，得到: %v", stor2.norecord())
	}
}

// TestFilePersistence 测试文件持久化
func TestFilePersistence(t *testing.T) {
	tempDir := t.TempDir()
	storageDir := filepath.Join(tempDir, "storage")

	// 第一次创建并保存
	fs1 := &FileStorage{
		dir:   storageDir,
		cache: make(map[int64]*GroupConfig),
	}

	if err := os.MkdirAll(storageDir, 0755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}

	gid := int64(777777)
	fs1.SaveRate(gid, 75)
	fs1.SaveTemp(gid, 85)
	fs1.SaveNoRecord(gid, true)

	// 检查文件是否存在
	filePath := filepath.Join(storageDir, "777777.json")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("配置文件应该存在: %s", filePath)
	}

	// 重新创建存储实例，模拟重启
	fs2 := &FileStorage{
		dir:   storageDir,
		cache: make(map[int64]*GroupConfig),
	}

	// 重新加载
	fs2.loadAll()

	// 检查配置是否保持
	config := fs2.Get(gid)
	if config.Rate != 75 {
		t.Errorf("重启后Rate应为75，得到: %d", config.Rate)
	}
	if config.Temp != 85 {
		t.Errorf("重启后Temp应为85，得到: %d", config.Temp)
	}
	if !config.NoRecord {
		t.Errorf("重启后NoRecord应为true，得到: %v", config.NoRecord)
	}
}
