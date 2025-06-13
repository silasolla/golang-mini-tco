package main

import (
    "go/ast"
    "go/parser"
    "go/printer"
    "go/token"
    "os"
)

// 末尾再帰か判定し，再帰呼び出しの return 文を探す
func findTailRecursionReturn(fn *ast.FuncDecl) (*ast.ReturnStmt, *ast.CallExpr) {
    var tailReturn *ast.ReturnStmt
    var call *ast.CallExpr

    // AST ノードの探索
    // true -> 別のノードを探索継続, false -> 探索終了
    ast.Inspect(fn.Body, func(n ast.Node) bool {
        // 現在のノード (interface) が return 文か確認
        ret, ok := n.(*ast.ReturnStmt)

        // return 文ではなかった
        // または戻り値が1つじゃない (単純化のためエラー処理の無い関数を想定)
        if !ok || len(ret.Results) != 1 {
            return true
        }

        // return 文の (第1) 戻り値が関数呼び出しか確認
        callExpr, ok := ret.Results[0].(*ast.CallExpr)

        // 関数呼び出しではなかった
        if !ok {
            return true
        }

        // 関数呼び出しの関数部分が識別子か確認
        funIdent, ok := callExpr.Fun.(*ast.Ident)

        // 識別子だった (無名関数とかじゃない)
        // かつ呼び出し先の関数名が呼び出し元の関数名と一致 (再帰)
        if ok && funIdent.Name == fn.Name.Name {
            // 発見した return 文と関数呼び出し式をメモ
            tailReturn = ret
            call = callExpr

            // 発見できたので探索終了
            return false 
        }

        // 末尾再帰じゃなかった
        return true
    })

    // 発見した return 文と関数呼び出し式を返す
    return tailReturn, call
}

// 引数リストを代入文に変換 (引数は Ident のみ想定)
func makeAssignStmts(params []*ast.Field, args []ast.Expr) []ast.Stmt {
    stmts := []ast.Stmt{}
    tmpNames := make([]*ast.Ident, 0, len(params)) // tmp変数名を記憶

    // 引数で渡していた式をすべて一時変数に代入する文
    for i, param := range params {
        if len(param.Names) == 0 {
            continue
        }
        tmpName := &ast.Ident{Name: "tmp" + param.Names[0].Name} // e.g. a -> tmpa, b -> tmpb
        tmpNames = append(tmpNames, tmpName)

        assignTmp := &ast.AssignStmt{
            Lhs: []ast.Expr{tmpName},
            Tok: token.DEFINE,
            Rhs: []ast.Expr{args[i]},
        }
        stmts = append(stmts, assignTmp)
    }

    // 一時変数から値を取り出して新しい状態を代入する文
    lhsIdents := make([]ast.Expr, 0, len(params))
    rhsIdents := make([]ast.Expr, 0, len(tmpNames))
    for i, param := range params {
        if len(param.Names) == 0 {
            continue
        }
        lhsIdents = append(lhsIdents, &ast.Ident{Name: param.Names[0].Name})
        rhsIdents = append(rhsIdents, tmpNames[i])
    }

    assignAll := &ast.AssignStmt{
        Lhs: lhsIdents,
        Tok: token.ASSIGN,
        Rhs: rhsIdents,
    }
    stmts = append(stmts, assignAll)

    return stmts
}

func main() {
    fset := token.NewFileSet()
    node, err := parser.ParseFile(fset, "input.go", nil, parser.ParseComments)
    if err != nil {
        panic(err)
    }

    for _, decl := range node.Decls {
        fn, ok := decl.(*ast.FuncDecl)
        if !ok || fn.Body == nil {
            continue
        }

        // 末尾再帰の return 文と関数呼び出しを探す
        tailRet, callExpr := findTailRecursionReturn(fn)
        if tailRet == nil || callExpr == nil {
            continue
        }

        // Base Step (if 文) をループ継続条件に使う
        var baseIf *ast.IfStmt
        if len(fn.Body.List) > 0 {
            ifStmt, ok := fn.Body.List[0].(*ast.IfStmt)
            if ok {
                baseIf = ifStmt
            }
        }
        if baseIf == nil {
            continue
        }

        // ループの条件を Base Step の否定として簡易的に変換
        // e.g. if n == 0 -> for n != 0
        var cond ast.Expr
        if be, ok := baseIf.Cond.(*ast.BinaryExpr); ok {
            switch be.Op {
            case token.EQL:
                cond = &ast.BinaryExpr{
                    X:  be.X,
                    Op: token.NEQ,
                    Y:  be.Y,
                }
            case token.NEQ:
                cond = &ast.BinaryExpr{
                    X:  be.X,
                    Op: token.EQL,
                    Y:  be.Y,
                }
            default:
                cond = baseIf.Cond
            }
        } else {
            cond = baseIf.Cond
        }

        // 再帰呼び出しの引数を代入文に変換
        assignStmts := makeAssignStmts(fn.Type.Params.List, callExpr.Args)

        // 再帰呼び出しの return 文だけ除外した残りの文を抽出
        var nonRecursiveStmts []ast.Stmt
        for _, stmt := range fn.Body.List {
            // baseIf 自体を除外
            if stmt == baseIf {
                continue
            }

            // 末尾再帰の return を除外
            retStmt, ok := stmt.(*ast.ReturnStmt)
            if ok && len(retStmt.Results) == 1 {
                if callExpr, ok := retStmt.Results[0].(*ast.CallExpr); ok {
                    if funIdent, ok := callExpr.Fun.(*ast.Ident); ok {
                        if funIdent.Name == fn.Name.Name {
                            // 末尾再帰呼び出しの return なので除外
                            continue
                        }
                    }
                }
            }
            nonRecursiveStmts = append(nonRecursiveStmts, stmt)
        }

        // for の外に置く return 文だけ取り出す
        var baseReturn *ast.ReturnStmt
        if len(baseIf.Body.List) > 0 {
            baseReturn, _ = baseIf.Body.List[0].(*ast.ReturnStmt)
        }

        // 新しい関数本文の生成
        fn.Body.List = []ast.Stmt{
            &ast.ForStmt{
                Cond: cond,
                Body: &ast.BlockStmt{
                    List: append(assignStmts, nonRecursiveStmts...), // 再帰 return は除外済み
                },
            },
            baseReturn, // ループのあとに return を置く (if は不要)
        }
    }

    printer.Fprint(os.Stdout, fset, node)
}

