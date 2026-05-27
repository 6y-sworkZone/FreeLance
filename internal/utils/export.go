package utils

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type HourExportEntry struct {
	Date        string
	Project     string
	Description string
	Duration    float64
	Rate        float64
	Amount      float64
}

func ExportHoursToCSV(entries []HourExportEntry, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	filename := fmt.Sprintf("hours_export_%s.csv", time.Now().Format("20060102_150405"))
	fullPath := filepath.Join(dir, filename)

	file, err := os.Create(fullPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	file.WriteString("\xEF\xBB\xBF")

	writer := csv.NewWriter(file)
	defer writer.Flush()

	writer.Write([]string{"日期", "项目", "描述", "工时(小时)", "时薪", "金额"})

	for _, e := range entries {
		writer.Write([]string{
			e.Date,
			e.Project,
			e.Description,
			fmt.Sprintf("%.2f", e.Duration),
			fmt.Sprintf("%.2f", e.Rate),
			fmt.Sprintf("%.2f", e.Amount),
		})
	}

	return fullPath, nil
}
