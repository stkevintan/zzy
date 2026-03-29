package resume

import (
	"fmt"
	"strconv"

	"github.com/xuri/excelize/v2"
)

var headers = []string{
	"序号",
	"联系结果",
	"姓名",
	"性别",
	"身份证号",
	"年龄",
	"民族",
	"籍贯",
	"政治面貌",
	"最高学历",
	"手机号码",
	"职称",
	"毕业院校（本科）",
	"本科专业",
	"毕业院校（硕士）",
	"硕士专业",
	"毕业时间",
	"教师资格证书类型",
	"现工作单位",
	"工作经验",
	"主要荣誉",
	"预计税前月薪\n（万/月）",
	"预计税前薪资\n（万/年）",
	"备注",
}

// ExportXLSX creates an xlsx file from a list of results and returns the raw bytes.
func ExportXLSX(results []Result, sheetName string) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()

	if sheetName == "" {
		sheetName = "简历汇总"
	}

	// Rename default sheet
	defaultSheet := f.GetSheetName(0)
	if err := f.SetSheetName(defaultSheet, sheetName); err != nil {
		return nil, fmt.Errorf("set sheet name: %w", err)
	}

	// --- styles ---
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 11},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
		Border: []excelize.Border{
			{Type: "left", Color: "000000", Style: 1},
			{Type: "top", Color: "000000", Style: 1},
			{Type: "right", Color: "000000", Style: 1},
			{Type: "bottom", Color: "000000", Style: 1},
		},
	})

	textStyle, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
		Border: []excelize.Border{
			{Type: "left", Color: "000000", Style: 1},
			{Type: "top", Color: "000000", Style: 1},
			{Type: "right", Color: "000000", Style: 1},
			{Type: "bottom", Color: "000000", Style: 1},
		},
	})

	cnyStyle, _ := f.NewStyle(&excelize.Style{
		CustomNumFmt: strPtr("￥#,##0.0000"),
		Alignment:    &excelize.Alignment{Vertical: "center"},
		Border: []excelize.Border{
			{Type: "left", Color: "000000", Style: 1},
			{Type: "top", Color: "000000", Style: 1},
			{Type: "right", Color: "000000", Style: 1},
			{Type: "bottom", Color: "000000", Style: 1},
		},
	})

	// --- headers ---
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheetName, cell, h)
		f.SetCellStyle(sheetName, cell, cell, headerStyle)
	}
	f.SetRowHeight(sheetName, 1, 30)

	// --- column widths ---
	colWidths := map[int]float64{
		1: 6, 2: 10, 3: 10, 4: 6, 5: 22, 6: 6, 7: 6, 8: 14,
		9: 10, 10: 10, 11: 14, 12: 10, 13: 18, 14: 18, 15: 18,
		16: 18, 17: 12, 18: 16, 19: 20, 20: 20, 21: 20, 22: 16,
		23: 16, 24: 14,
	}
	for col, w := range colWidths {
		colName, _ := excelize.ColumnNumberToName(col)
		f.SetColWidth(sheetName, colName, colName, w)
	}

	// --- data rows ---
	for i, r := range results {
		if r.Error != "" {
			continue
		}
		row := i + 2 // row 1 is header
		e := r.Entry

		values := []any{
			i + 1,                                // 序号
			"",                                   // 联系结果
			string(e.Name),                       // 姓名
			string(e.Gender),                     // 性别
			e.IDNumber,                           // 身份证号
			e.Age,                                // 年龄
			e.Ethnicity,                          // 民族
			e.NativePlace,                        // 籍贯
			string(e.PoliticalStatus),            // 政治面貌
			string(e.HighestEducation),           // 最高学历
			e.PhoneNumber,                        // 手机号码
			string(e.ProfessionalTitle),          // 职称
			e.UndergraduateSchool,                // 毕业院校（本科）
			e.UndergraduateMajor,                 // 本科专业
			e.GraduateSchool,                     // 毕业院校（硕士）
			e.GraduateMajor,                      // 硕士专业
			e.GraduationDate,                     // 毕业时间
			string(e.TeachingCertificate),        // 教师资格证书类型
			e.CurrentEmployer,                    // 现工作单位
			e.WorkExperience,                     // 工作经验
			e.MainHonors,                         // 主要荣誉
			parseSalary(e.ExpectedMonthlySalary), // 预计税前月薪
			parseSalary(e.ExpectedAnnualSalary),  // 预计税前薪资
			e.Remarks,                            // 备注
		}

		for j, v := range values {
			cell, _ := excelize.CoordinatesToCellName(j+1, row)
			f.SetCellValue(sheetName, cell, v)

			// Apply CNY style for salary columns (22, 23)
			if j == 21 || j == 22 {
				f.SetCellStyle(sheetName, cell, cell, cnyStyle)
			} else {
				f.SetCellStyle(sheetName, cell, cell, textStyle)
			}
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write xlsx: %w", err)
	}
	return buf.Bytes(), nil
}

func parseSalary(s string) float64 {
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

func strPtr(s string) *string {
	return &s
}
