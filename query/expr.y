%{
package query
%}

%union {
	str string
	expr Expr
	list []Expr
}

%token <str> '(' ')' ','

/* The 'p' is for privacy */
%token <str> pNUMBER
%token <str> pWORD
%token <str> pSTRING

/* it was easier to recognize metrics
  in the lexer than here in the parser */
%token <str> pMETRIC

%token <str> pERROR /* not used */

%type <expr> function query expr
%type <list> arglist
%%
top: query { yylex.(*lexer).result = Query{Expr: $1} }

/* A query consists of a single metric pattern or a single
  function call. Numbers and quoted strings are not allowed
  at the top level. */
query:
	pMETRIC { $$ = Metric($1) }
|	function

function:
	pWORD '(' arglist ')'
	{
		$$ = Func{Name: $1, Args: $3}
	}

arglist:
	/* empty */      { $$ = nil }
|	expr             { $$ = append($$, $1) }
|	arglist ',' expr { $$ = append($1, $3) }

expr:
	query   { $$ = $1 }
|	pSTRING { $$ = Value($1) }
|	pNUMBER { $$ = Value($1) }
