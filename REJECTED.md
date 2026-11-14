

# Decisions or options that have been rejected
- choice of money package. There are popular open source packages for golang such as shopspring/decimal and go-money
they could have been used but i didn't want to add more complexity to the project that it needed to be.
So that left us with an int64 as a choice for our money minor units scale. It can overflow and we have tests that covered that.