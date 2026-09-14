package domain

type Pagination struct {
	Page             int
	PageSize         int
	TotalPage        int
	TotalRecords     int
	CurrentPageTotal int
}
