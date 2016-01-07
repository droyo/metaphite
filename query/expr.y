%{
package query

%}

%union {
	val Value
	metric Metric
	function Func
	query Query
	expr Expr
	exprs []Expr
}

%token <val> WORD
%token <val> VALUE ',' '(' ')' '.' '{' '}'

%type <query> query
%type <function> function
%type <exprs> arglist
%type <expr> expression
%type <metric> metric
%type <val> wordlist

%%
top: query { yylex.(*lexer).result = $1 }

query:
	metric
	{
		$$ = Query{Expr: $1}
	}
|	function
	{
		$$ = Query{Expr: $1}
	}

function:
	WORD '('  arglist ')'
	{
		$$ = Func{Name: string($1), Args: $3}
	}

arglist:
	/* empty */
	{
		$$ = nil
	}
|	expression
	{
		$$ = append($$, $1)
	}
|	arglist ',' expression
	{
		$$ = append($1, $3)
	}

expression:
	query
	{
		$$ = $1
	}
|	metric
	{
		$$ = $1
	}
|	VALUE
	{
		$$ = $1
	}

metric:
	WORD
	{
		$$ = Metric($1)
	}
|	metric '.' WORD
	{
		$$ = Metric(string($1) + "." + string($3))
	}
|	metric '{' wordlist '}'
	{
		$$ = Metric(string($1) + "{" + string($3) + "}")
	}

wordlist:
	WORD
|	wordlist ',' WORD
	{
		$$ = $1 + "," + $3
	}
|	wordlist ','
	{
		$$ = $1 + ","
	}
