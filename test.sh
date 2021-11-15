# 按照https://github.com/UoB-CSA/gol-skeleton步骤执行测试函数
echo 'test step1'
go test -v -run=TestGol/-1$

echo 'test step2'
go test -v -run=TestGol

echo 'test step3'
go test -v -run=TestAlive

echo 'test step4'
go test -v -run=TestPgm