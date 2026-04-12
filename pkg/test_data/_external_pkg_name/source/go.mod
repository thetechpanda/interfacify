module example.com/interfacify-externalpkg/source

go 1.26.1

require example.com/interfacify-externalpkg/dep/v3 v3.0.0

replace example.com/interfacify-externalpkg/dep/v3 => ../dep
