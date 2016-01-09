%{
package query
%}

%union {
	str  string
	box  Expr
	list []Expr
}

%token <str> WORD   /* unquoted word or number */
%token <str> NUMBER
%token <str> STRING /* quoted string literal */
%token <str> ',' '(' ')' '.' '{' '}'

%type <box>  expression
%type <box>  function
%type <box>  metric
%type <box>  query

%type <list> arglist
%type <str>  bracelist
%type <str>  wordlist
%type <str> pathelem

%%
top: query { yylex.(*lexer).result = Query{Expr: $1} }

query:
	metric
|	function

function:
	WORD '(' arglist ')'
	{
		$$ = Func{string($1), $3}
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

metric:
	pathelem
	{
		$$ = Metric($1)
	}
|	metric '.' pathelem
	{
		$$ = Metric(string($1.(Metric)) + "." + $3)
	}

pathelem:
	WORD
|	bracelist
	{
		$$ = $1
	}
|	WORD bracelist
	{
		$$ = $1 + $2
	}
|	bracelist WORD
	{
		$$ = $1 + $2
	}

expression:
	STRING
	{
		$$ = Value($1)
	}
|	query

wordlist:
	/* empty */
	{
		$$ = ""
	}
|	WORD
	{
		$$ = $1
	}
|	wordlist ',' WORD
	{
		$$ = $1 + "," + $3
	}
|	wordlist ','
	{
		$$ = $1 + ","
	}

bracelist:
	'{' wordlist '}'
	{
		$$ = "{" + $2 + "}"
	}
