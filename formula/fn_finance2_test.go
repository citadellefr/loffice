package formula

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// shown is the number of decimals a documented value is written with.
func shown(want string) float64 {
	_, decimals, _ := strings.Cut(want, ".")
	return 0.5 * math.Pow(10, -float64(len(decimals)))
}

func TestFinance2(t *testing.T) {
	b := book(map[string]any{
		"A1": -10000, "A2": 2750, "A3": 4250, "A4": 3250, "A5": 2750,
		"B1": "=", "C1": 39448, "C2": 39508, "C3": 39751, "C4": 39859, "C5": 39904,
		"D1": -120000, "D2": 39000, "D3": 30000, "D4": 21000, "D5": 37000, "D6": 46000,
		"E1": 0.09, "E2": 0.11, "E3": 0.1,
	})
	for _, c := range []struct{ f, want string }{
		{`XNPV(0.09,A1:A5,C1:C5)`, "2086.647602"},
		{`XIRR(A1:A5,C1:C5,0.1)`, "0.37336253"},
		{`XIRR(A1:A5,C1:C5)`, "0.37336253"},
		{`MIRR(D1:D6,0.1,0.12)`, "0.126094"},
		{`MIRR(D1:D4,0.1,0.12)`, "-0.048045"},
		{`MIRR(D1:D6,0.1,0.14)`, "0.134759"},
		{`DB(1000000,100000,6,1,7)`, "186083.33"},
		{`DB(1000000,100000,6,2,7)`, "259639.42"},
		{`DB(1000000,100000,6,3,7)`, "176814.44"},
		{`DB(1000000,100000,6,4,7)`, "120410.64"},
		{`DB(1000000,100000,6,5,7)`, "81999.64"},
		{`DB(1000000,100000,6,6,7)`, "55841.76"},
		{`DB(1000000,100000,6,7,7)`, "15845.10"},
		{`DDB(2400,300,10*365,1)`, "1.32"},
		{`DDB(2400,300,10*12,1)`, "40.00"},
		{`DDB(2400,300,10,1)`, "480.00"},
		{`DDB(2400,300,10,2,1.5)`, "306.00"},
		{`DDB(2400,300,10,10)`, "22.12"},
		{`SYD(30000,7500,10,1)`, "4090.91"},
		{`SYD(30000,7500,10,10)`, "409.09"},
		{`VDB(2400,300,10*365,0,1)`, "1.32"},
		{`VDB(2400,300,10*12,0,1)`, "40.00"},
		{`VDB(2400,300,10,0,1)`, "480.00"},
		{`VDB(2400,300,10*12,6,18)`, "396.31"},
		{`VDB(2400,300,10*12,6,18,3)`, "540.19"},
		{`CUMIPMT(0.09/12,30*12,125000,13,24,0)`, "-11135.23213"},
		{`CUMIPMT(0.09/12,30*12,125000,1,1,0)`, "-937.5"},
		{`CUMPRINC(0.09/12,30*12,125000,13,24,0)`, "-934.1071"},
		{`CUMPRINC(0.09/12,30*12,125000,1,1,0)`, "-68.27827"},
		{`RRI(96,10000,11000)`, "0.000993"},
		{`FVSCHEDULE(1,E1:E3)`, "1.33089"},
		{`FVSCHEDULE(1,{0.09,0.11,0.1})`, "1.33089"},
		{`EFFECT(0.0525,4)`, "0.053543"},
		{`NOMINAL(0.053543,4)`, "0.052500"},
		{`DOLLARDE(1.02,16)`, "1.125"},
		{`DOLLARDE(1.1,32)`, "1.3125"},
		{`DOLLARFR(1.125,16)`, "1.02"},
		{`DOLLARFR(1.125,32)`, "1.04"},
		{`PRICE(DATE(2008,2,15),DATE(2017,11,15),0.0575,0.065,100,2,0)`, "94.63436162"},
		{`YIELD(DATE(2008,2,15),DATE(2016,11,15),0.0575,95.04287,100,2,0)`, "0.065"},
		{`PRICEDISC(DATE(2008,2,16),DATE(2008,3,1),0.0525,100,2)`, "99.79583333"},
		{`YIELDDISC(DATE(2008,2,16),DATE(2008,3,1),99.795,100,2)`, "0.052823"},
		{`DISC(DATE(2007,1,25),DATE(2007,6,15),97.975,100,1)`, "0.052420"},
		{`INTRATE(DATE(2008,2,15),DATE(2008,5,15),1000000,1014420,2)`, "0.05768"},
		{`RECEIVED(DATE(2008,2,15),DATE(2008,5,15),1000000,0.0575,2)`, "1014584.654"},
		{`TBILLEQ(DATE(2008,3,31),DATE(2008,6,1),0.0914)`, "0.094151"},
		{`TBILLPRICE(DATE(2008,3,31),DATE(2008,6,1),0.09)`, "98.45"},
		{`TBILLYIELD(DATE(2008,3,31),DATE(2008,6,1),98.45)`, "0.091417"},
		{`ACCRINT(DATE(2008,3,1),DATE(2008,8,31),DATE(2008,5,1),0.1,1000,2,0)`, "16.66667"},
		{`ACCRINT(DATE(2008,3,5),DATE(2008,8,31),DATE(2008,5,1),0.1,1000,2,0)`, "15.55556"},
		{`ACCRINTM(DATE(2008,4,1),DATE(2008,6,15),0.1,1000,3)`, "20.54794521"},
		{`COUPDAYBS(DATE(2011,1,25),DATE(2011,11,15),2,1)`, "71"},
		{`COUPDAYS(DATE(2011,1,25),DATE(2011,11,15),2,1)`, "181"},
		{`COUPDAYSNC(DATE(2011,1,25),DATE(2011,11,15),2,1)`, "110"},
		{`COUPNCD(DATE(2011,1,25),DATE(2011,11,15),2,1)`, "40678"},
		{`COUPPCD(DATE(2011,1,25),DATE(2011,11,15),2,1)`, "40497"},
		{`COUPNUM(DATE(2007,1,25),DATE(2008,11,15),2,1)`, "4"},
		{`DURATION(DATE(2008,1,1),DATE(2016,1,1),0.08,0.09,2,1)`, "5.993775"},
		{`MDURATION(DATE(2008,1,1),DATE(2016,1,1),0.08,0.09,2,1)`, "5.73567"},
		{`NETWORKDAYS.INTL(DATE(2006,1,1),DATE(2006,1,31))`, "22"},
		{`NETWORKDAYS.INTL(DATE(2006,2,28),DATE(2006,1,31))`, "-21"},
		{`NETWORKDAYS.INTL(DATE(2006,1,1),DATE(2006,1,31),11)`, "26"},
		{`NETWORKDAYS.INTL(DATE(2006,1,1),DATE(2006,1,31),"0000011")`, "22"},
		{`NETWORKDAYS.INTL(DATE(2006,1,1),DATE(2006,1,31),"0011000")`, "23"},
		{`NETWORKDAYS.INTL(DATE(2006,1,1),DATE(2006,1,31),7,DATE(2006,1,2))`, "22"},
		{`WORKDAY.INTL(DATE(2012,1,1),30,0)`, "#NUM!"},
		{`WORKDAY.INTL(DATE(2012,1,1),90,11)`, "41013"},
		{`WORKDAY.INTL(DATE(2012,1,1),30,17)`, "40944"},
		{`WORKDAY.INTL(DATE(2012,1,1),-5,"0000011")`, "40903"},
		{`WORKDAY.INTL(DATE(2012,1,1),5,"1111111")`, "#VALUE!"},
		{`ISOWEEKNUM(DATE(2012,3,9))`, "10"},
		{`DAYS(DATE(2021,3,15),DATE(2021,2,1))`, "42"},
		{`XIRR({1,2},{40000,40100})`, "#NUM!"},
		{`XIRR({-1,-2},{40000,40100})`, "#NUM!"},
		{`MIRR({1,2,3},0.1,0.1)`, "#DIV/0!"},
		{`PRICE(DATE(2008,2,15),DATE(2008,2,15),0.05,0.06,100,2)`, "#NUM!"},
		{`PRICE(DATE(2008,2,15),DATE(2009,2,15),0.05,0.06,100,3)`, "#NUM!"},
		{`TBILLPRICE(DATE(2008,3,31),DATE(2009,6,1),0.09)`, "#NUM!"},
		{`DOLLARDE(1.02,0)`, "#DIV/0!"},
		{`DOLLARFR(1.02,-1)`, "#NUM!"},
	} {
		got := eval(t, b, English, c.f)
		if strings.HasPrefix(c.want, "#") {
			if got.show() != c.want {
				t.Errorf("%s = %s, want %s", c.f, got.show(), c.want)
			}
			continue
		}
		var want float64
		if _, err := fmt.Sscan(c.want, &want); err != nil || got.Type != TypeNumber || math.Abs(got.Num-want) > shown(c.want) {
			t.Errorf("%s = %s, want %s", c.f, got.show(), c.want)
		}
	}
}

func TestYieldInvertsPrice(t *testing.T) {
	b := book(map[string]any{})
	for _, basis := range []string{"0", "1", "2", "3", "4"} {
		for _, freq := range []string{"1", "2", "4"} {
			for _, mat := range []string{"DATE(2009,3,31)", "DATE(2016,11,15)", "DATE(2008,9,1)", "DATE(2038,2,28)"} {
				price := eval(t, b, English, `PRICE(DATE(2008,2,15),`+mat+`,0.0575,0.071,100,`+freq+`,`+basis+`)`)
				got := eval(t, b, English, `YIELD(DATE(2008,2,15),`+mat+`,0.0575,`+price.show()+`,100,`+freq+`,`+basis+`)`)
				if got.Type != TypeNumber || math.Abs(got.Num-0.071) > 1e-8 {
					t.Errorf("YIELD of PRICE %s (%s, %s, %s) = %s, want 0.071", price.show(), mat, freq, basis, got.show())
				}
			}
		}
	}
}

func TestXIRRIsXNPVRoot(t *testing.T) {
	b := book(map[string]any{})
	for _, f := range []string{
		`{-100,10,10,10,10,10},{40000,40365,40730,41095,41461,41826}`,
		`{-100,1,1},{40000,40365,40730}`,
		`{500,-100,-100,-100},{40000,40100,40200,40300}`,
		`{-1000,100,5000},{40000,40001,50000}`,
	} {
		r := eval(t, b, English, `XIRR(`+f+`)`)
		if r.Type != TypeNumber {
			t.Errorf("XIRR(%s) = %s", f, r.show())
			continue
		}
		if npv := eval(t, b, English, `XNPV(`+r.show()+`,`+f+`)`); math.Abs(npv.Num) > 1e-6 {
			t.Errorf("XNPV at XIRR(%s) = %s", f, npv.show())
		}
	}
}

func TestIterations(t *testing.T) {
	b := book(map[string]any{})
	for f, want := range map[string]string{
		`XIRR({-100,60,60},{40000,40365,40730})`:                     "0.13066239",
		`XIRR({-100,60,60},{40000,40365,40730},-0.5)`:                "0.13066239",
		`XIRR({-100,60,60},{40000,40365,40730},-1)`:                  "#NUM!",
		`XIRR({-100,60,60},{40365,40000,40730})`:                     "#NUM!",
		`XIRR({-100,60,60},{40000,40365})`:                           "#NUM!",
		`YIELD(DATE(2008,2,15),DATE(2016,11,15),0.0575,0,100,2,0)`:   "#NUM!",
		`PRICE(DATE(2008,2,15),DATE(2016,11,15),-0.01,0.05,100,2,0)`: "#NUM!",
		`PRICE(DATE(2008,2,15),DATE(2016,11,15),0.01,-0.05,100,2,0)`: "#NUM!",
		`DB(1000,100,5,6,12)`:                                        "#NUM!",
		`CUMIPMT(0.01,12,100,5,3,0)`:                                 "#NUM!",
		`CUMIPMT(0.01,12,100,1,13,0)`:                                "#NUM!",
		`CUMPRINC(0.01,12,100,1,12,2)`:                               "#NUM!",
		`RRI(0,1,2)`:                                                 "#NUM!",
		`FVSCHEDULE(1,{"a"})`:                                        "1",
		`NETWORKDAYS.INTL(DATE(2006,1,1),DATE(2006,1,31),18)`:        "#NUM!",
		`NETWORKDAYS.INTL(DATE(2006,1,1),DATE(2006,1,31),"1111111")`: "#VALUE!",
	} {
		got := eval(t, b, English, f)
		if strings.HasPrefix(want, "#") {
			if got.show() != want {
				t.Errorf("%s = %s, want %s", f, got.show(), want)
			}
			continue
		}
		var w float64
		fmt.Sscan(want, &w)
		if got.Type != TypeNumber || math.Abs(got.Num-w) > shown(want) {
			t.Errorf("%s = %s, want %s", f, got.show(), want)
		}
	}
}
