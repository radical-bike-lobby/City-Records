package main

var fixedRecords = map[string]Record{
	"AeqI9D7qFU3i0kxGEGEOoxYXYcM5LmbMaNMWHYNSOluuAluvrrDrfpACVhoyakW5V1eVÁsÉiIdGdBZbzdRCbYÁ4=": Record{
		Name: "Contract - # 10183 - Date Executed:  -",
		DisplayColumnValues: []DisplayColumnValue{
			{},
			{Value: "10183"},
			{Value: "9/14/2016"},
			{},
		},
	},
	"AbVth5BEK8HOIB3x469uSÉK5eÁSJeFt0HvXDvd4WHZ6dm5ÁA9I8D5E5jÁ69Áb38w5NpgYIsTLdYZtoHOQDUal74=": Record{
		Name: "Contract - # 10183 - Date Executed:  -",
		DisplayColumnValues: []DisplayColumnValue{
			{},
			{Value: "10183"},
			{Value: "9/14/2016"},
			{},
		},
	},
}

// map of record ids to ignore to the message returned by city records api due to failure
var ignoredIds = map[string]string{
	// "AduVnsWXoCvHupViNhmqognjxARch0wRÁ5jpÉÁL0dNXÁhRoÁSPyUtheI1UrqkBÉYmt8CakNZhMrmLatqLj4qeYk=": `500. Body: {"Message":"The format of value '10/25/2005; CLK - Agenda Packet (Public); District 6; 0; ; Amending \"By Right\" Residential Additions and Definition of a Story in the Zoning Ordinance.pdf' is invalid."}`,
	// "AQyVFdak5uemFd9vVl3VxZbTl4AypjT3Ke3afptqnYj5QXKFRKdCKdgNdiL1aCHzpDA8mYwFzazlO7HBmtMwXtc=": `500. Body: {"Message":"The format of value '4/26/1983; CLK - Resolution; City Council; 51751; ; Authorizing Allocation of 9th Year Community Development Block Grant Funds to YMCA \"New Light\" Senior Center.pdf' is invalid."}`,
	// "AU5O7awJhrmÉjqmCHSr6bbÁRCzQy4ÁAÉSHwKlLZrU2zYUFPH7oGJoEZbÁz14lHFEceLppVrpGXN5AEh75UQP8MI=": `500. Body: {"Message":"The format of value '11/15/2005; CLK - Agenda Packet (Public); Planning and Development; 0; ; Amending \"By Right\"  Residential Additions and Definition of a Story in Zoning Ordinance.pdf' is invalid."}`,
	// "AeAtuYDfQN9PKsONN7uyaV0x7uyJd9ikPS68TBHBkSDjo8wKcpirDKXZ91hjsOUÁ3C7lÁxHM7O2PYDXc3Á2DP6Y=": `500. Body: {"Message":"The format of value '10/25/2005; CLK - Agenda Packet (Public); District 6; 0; ; Amending \"By Right\" Residential Additions and Definition of a Story in the Zoning Ordinance.pdf' is invalid."}`,
	// "AflxP39Á1ÉrqX94rJDtAÁfUP92WLzVÁ5HDÉRFxpmgfkMjjw5amÁ1hzgdKXrG38pmVRZxiBnqETepZptf63edhHM=": `500. Body: {"Message":"The format of value '4/26/1983; CLK - Resolution; City Council; 51751; ; Authorizing Allocation of 9th Year Community Development Block Grant Funds to YMCA \"New Light\" Senior Center.pdf' is invalid."}`,
	// "AQkvgCzd6o7Étdv4rZVNGLo6yBzqTwV0c6dEÉ98EEÁ75t0jRRnZxWbIUQEpLlR7RV0ÁEYNSjÉz0ycOABgBhRi28=": `500. Body: {"Message":"The format of value '10/25/2005; CLK - Agenda Packet (Public); District 6; 0; ; Amending \"By Right\" Residential Additions and Definition of a Story in the Zoning Ordinance.pdf' is invalid."}`,
}
