
# Worklog

So so far it was just setting up the repo with golang (1.27) and adding golanci as well

- First actual work is money.go part. It has 4 methods (Decimal, String, Add, Neg). They are covered with unit tests.
- One decision is to use int64 for minor. The problem here is that this can overflow. We have updated in the REJECTED.md why other packages were rejected.