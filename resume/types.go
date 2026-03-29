package resume

// Subject 学科
type Subject string

const (
	SubjectChinese   Subject = "语文"
	SubjectMath      Subject = "数学"
	SubjectEnglish   Subject = "英语"
	SubjectPhysics   Subject = "物理"
	SubjectChemistry Subject = "化学"
	SubjectPolitics  Subject = "政治"
	SubjectBiology   Subject = "生物"
	SubjectGeography Subject = "地理"
	SubjectPE        Subject = "体育"
	SubjectMusic     Subject = "音乐"
	SubjectArt       Subject = "美术"
	SubjectDorm      Subject = "宿管"
	SubjectOther     Subject = "其他"
)

// Gender 性别
type Gender string

const (
	GenderMale   Gender = "男"
	GenderFemale Gender = "女"
)

// PoliticalStatus 政治面貌
type PoliticalStatus string

const (
	PoliticalStatusPartyMember        PoliticalStatus = "党员"
	PoliticalStatusProbationaryMember PoliticalStatus = "预备党员"
	PoliticalStatusCitizen            PoliticalStatus = "群众"
	PoliticalStatusLeagueMember       PoliticalStatus = "团员"
)

// Education 最高学历
type Education string

const (
	EducationBachelor Education = "本科"
	EducationMaster   Education = "硕士"
	EducationDoctor   Education = "博士"
)

// ProfessionalTitle 职称
type ProfessionalTitle string

const (
	ProfessionalTitleNone       ProfessionalTitle = "无"
	ProfessionalTitleSecondary  ProfessionalTitle = "二级教师"
	ProfessionalTitleFirstClass ProfessionalTitle = "一级教师"
	ProfessionalTitleSenior     ProfessionalTitle = "高级"
	ProfessionalTitleSuper      ProfessionalTitle = "正高级"
)

// TeachingCertificateType 教师资格证书类型
type TeachingCertificateType string

const (
	TeachingCertificateElementary TeachingCertificateType = "小学"
	TeachingCertificateMiddle     TeachingCertificateType = "初中"
	TeachingCertificateHighSchool TeachingCertificateType = "高中"
	TeachingCertificateVocational TeachingCertificateType = "中职"
)

// ResumeEntry 简历信息表
type ResumeEntry struct {
	Subject               Subject                 `json:"学科"`
	Name                  string                  `json:"姓名"`
	Gender                Gender                  `json:"性别"`
	IDNumber              string                  `json:"身份证号"`
	Age                   int                     `json:"年龄"`
	Ethnicity             string                  `json:"民族"`
	NativePlace           string                  `json:"籍贯"`
	PoliticalStatus       PoliticalStatus         `json:"政治面貌"`
	HighestEducation      Education               `json:"最高学历"`
	PhoneNumber           string                  `json:"手机号码"`
	ProfessionalTitle     ProfessionalTitle       `json:"职称"`
	WorkStartDate         string                  `json:"工作年月"`
	UndergraduateSchool   string                  `json:"毕业院校_本科"`
	UndergraduateMajor    string                  `json:"本科专业"`
	GraduateSchool        string                  `json:"毕业院校_硕士"`
	GraduateMajor         string                  `json:"硕士专业"`
	GraduationDate        string                  `json:"毕业时间"`
	TeachingCertificate   TeachingCertificateType `json:"教师资格证书类型"`
	CurrentEmployer       string                  `json:"现工作单位"`
	WorkExperience        string                  `json:"工作经验"`
	MainHonors            string                  `json:"主要荣誉"`
	ExpectedMonthlySalary string                  `json:"预计税前月薪_万每月"`
	ExpectedAnnualSalary  string                  `json:"预计税前薪资_万每年"`
	Remarks               string                  `json:"备注"`
}
