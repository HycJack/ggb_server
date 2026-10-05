package main

import (
	"fmt"
	"ggb_server/internal/app/model"
	"ggb_server/internal/pkg/database"
	"ggb_server/internal/repository"
	"github.com/spf13/viper"
	"github.com/xuri/excelize/v2"
	"log"
	"os"
	"path/filepath"
	"time"
)

var prefix string

func init() {
	ipMap := map[string]string{
		"dev":  "106.52.162.78",
		"prod": "120.48.91.203",
	}
	type Database struct {
		Host         string `mapstructure:"host"`
		Port         int    `mapstructure:"port"`
		User         string `mapstructure:"user"`
		Password     string `mapstructure:"password"`
		Name         string `mapstructure:"name"`
		MaxIdleConns int    `mapstructure:"max_idle_conns"`
		MaxOpenConns int    `mapstructure:"max_open_conns"`
	}
	type Server struct {
		Port int    `mapstructure:"port"`
		Mode string `mapstructure:"mode"`
	}
	viper.AddConfigPath("..")
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		panic(err)
	}

	var d Database
	var s Server
	viper.UnmarshalKey("server", &s)
	prefix = fmt.Sprintf("http://%s:%d", ipMap[s.Mode], s.Port)
	viper.UnmarshalKey("db", &d)
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
		d.User,
		d.Password,
		d.Host,
		d.Port,
		d.Name)
	err := database.InitDB(dsn)
	if err != nil {
		panic(err)
	}
}

// ExcelExporter 导出消息数据到Excel
type ExcelExporter struct {
	OutputDir string
}

// NewExcelExporter 创建新的Excel导出器
func NewExcelExporter(outputDir string) *ExcelExporter {
	return &ExcelExporter{
		OutputDir: outputDir,
	}
}

// Export 导出消息数据到Excel文件
func (e *ExcelExporter) Export(messages []model.Message) (string, error) {
	// 创建新的Excel文件
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Printf("警告：关闭Excel文件时出错: %v\n", err)
		}
	}()

	// 设置工作表
	sheetName := "消息记录"
	if err := setupSheet(f, sheetName); err != nil {
		return "", fmt.Errorf("设置工作表失败: %w", err)
	}

	// 设置表头样式
	headerStyle, err := createHeaderStyle(f)
	if err != nil {
		return "", fmt.Errorf("创建表头样式失败: %w", err)
	}

	// 设置数据样式
	dataStyle, err := createDataStyle(f)
	if err != nil {
		return "", fmt.Errorf("创建数据样式失败: %w", err)
	}

	// 写入表头
	if err := writeHeaders(f, sheetName, headerStyle); err != nil {
		return "", fmt.Errorf("写入表头失败: %w", err)
	}

	// 写入数据
	if err := writeData(f, sheetName, messages, dataStyle); err != nil {
		return "", fmt.Errorf("写入数据失败: %w", err)
	}

	// 设置列属性
	setColumnProperties(f, sheetName)

	// 设置文档属性
	setDocumentProperties(f)

	// 确保输出目录存在
	if err := e.ensureOutputDir(); err != nil {
		return "", fmt.Errorf("准备输出目录失败: %w", err)
	}

	// 生成文件路径
	filePath := filepath.Join(e.OutputDir, generateFilename())

	// 保存文件
	if err := f.SaveAs(filePath); err != nil {
		return "", fmt.Errorf("保存文件失败: %w", err)
	}

	return filePath, nil
}

// setupSheet 设置工作表
func setupSheet(f *excelize.File, sheetName string) error {
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return err
	}
	f.SetActiveSheet(index)

	// 删除默认的Sheet1
	if err := f.DeleteSheet("Sheet1"); err != nil {
		return err
	}

	return nil
}

// createHeaderStyle 创建表头样式
func createHeaderStyle(f *excelize.File) (int, error) {
	styleID, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF", Size: 12},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"4F81BD"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border: []excelize.Border{
			{Type: "left", Color: "000000", Style: 1},
			{Type: "top", Color: "000000", Style: 1},
			{Type: "bottom", Color: "000000", Style: 1},
			{Type: "right", Color: "000000", Style: 1},
		},
	})
	return styleID, err
}

// createDataStyle 创建数据样式
func createDataStyle(f *excelize.File) (int, error) {
	styleID, err := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{
			WrapText:    true,
			ShrinkToFit: false,
			Vertical:    "top",
		},
		Border: []excelize.Border{
			{Type: "left", Color: "DDDDDD", Style: 1},
			{Type: "top", Color: "DDDDDD", Style: 1},
			{Type: "bottom", Color: "DDDDDD", Style: 1},
			{Type: "right", Color: "DDDDDD", Style: 1},
		},
	})
	return styleID, err
}

// writeHeaders 写入表头
func writeHeaders(f *excelize.File, sheetName string, styleID int) error {
	headers := []struct {
		title  string
		width  float64
		column string
	}{
		{"用户消息", 40, "A"},
		{"图片路径", 30, "B"},
		{"意图识别", 20, "C"},
		{"提取", 20, "D"},
		{"生成ggb", 20, "E"},
		{"生成html", 20, "F"},
	}

	for _, h := range headers {
		cell := fmt.Sprintf("%s1", h.column)
		f.SetCellValue(sheetName, cell, h.title)
		f.SetCellStyle(sheetName, cell, cell, styleID)
		f.SetColWidth(sheetName, h.column, h.column, h.width)
	}

	// 设置表头行高
	//f.SetRowHeight(sheetName, 1, 25)

	return nil
}

// writeData 写入数据
func writeData(f *excelize.File, sheetName string, messages []model.Message, styleID int) error {
	row := 2
	for _, msg := range messages {
		// 获取资源URL
		resourceURL := getFirstResourceURL(msg.Resources)

		// 按阶段分类AI消息
		stageMessages := map[int]string{
			0: "", // 意图识别
			1: "", // 提取
			2: "", // 生成ggb
			3: "", // 生成html
		}

		// 收集各阶段的AI消息
		for _, aiMsg := range msg.AiMessages {
			if aiMsg.Stage >= 0 && aiMsg.Stage <= 3 {
				stageMessages[aiMsg.Stage] = aiMsg.Message
			}
		}

		// 写入行数据
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), msg.Message)        // 用户消息
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), prefix+resourceURL) // 图片路径
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), stageMessages[0])   // 意图识别
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), stageMessages[1])   // 提取
		f.SetCellValue(sheetName, fmt.Sprintf("E%d", row), stageMessages[2])   // 生成ggb
		f.SetCellValue(sheetName, fmt.Sprintf("F%d", row), stageMessages[3])   // 生成html

		// 应用样式
		for _, col := range []string{"A", "B", "C", "D", "E", "F"} {
			cell := fmt.Sprintf("%s%d", col, row)
			f.SetCellStyle(sheetName, cell, cell, styleID)
		}

		// 设置行高
		f.SetRowHeight(sheetName, row, 50)

		row++
	}
	return nil
}

// setColumnProperties 设置列属性
func setColumnProperties(f *excelize.File, sheetName string) {
	// 已在writeHeaders中设置列宽
	// 这里可以添加其他列属性设置
}

// setDocumentProperties 设置文档属性
func setDocumentProperties(f *excelize.File) {
	f.SetDocProps(&excelize.DocProperties{
		Creator:  "消息导出系统",
		Created:  time.Now().Format(time.RFC3339),
		Modified: time.Now().Format(time.RFC3339),
		Version:  "1.0",
	})
}

// getFirstResourceURL 获取第一条资源的URL
func getFirstResourceURL(resources []*model.Resource) string {
	if len(resources) > 0 && resources[0] != nil {
		return resources[0].URL
	}
	return ""
}

// generateFilename 生成文件名
func generateFilename() string {
	return fmt.Sprintf("messages_export_%s.xlsx", time.Now().Format("20060102_150405"))
}

// ensureOutputDir 确保输出目录存在
func (e *ExcelExporter) ensureOutputDir() error {
	return os.MkdirAll(e.OutputDir, 0755)
}

func main() {
	db := database.GetDB()
	msgRepo := repository.NewMessageRepository[model.Message]()
	messages, err := msgRepo.GetAiMessage(db)
	if err != nil {
		panic(err)
	}

	// 创建导出器并导出数据
	exporter := NewExcelExporter("exports")
	filePath, err := exporter.Export(messages)
	if err != nil {
		log.Fatalf("导出失败: %v", err)
	}

	fmt.Printf("成功导出到文件: %s\n", filePath)
}
