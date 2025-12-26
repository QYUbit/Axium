package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"reflect"
	"strings"
)

// ComponentInfo speichert geparste Informationen über eine Komponente
type ComponentInfo struct {
	Name        string
	TypeName    string
	IsRead      bool
	IsWrite     bool
	IsSingleton bool
}

// ScopeInfo repräsentiert einen geparsten Scope
type ScopeInfo struct {
	Name       string
	Components []ComponentInfo
}

func main() {
	filename := "example.go"

	// Parse die Go-Datei
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		log.Fatal(err)
	}

	// Finde alle Scopes
	scopes := findScopes(file)

	// Generiere Code
	for _, scope := range scopes {
		generateCode(scope)
	}
}

func findScopes(file *ast.File) []ScopeInfo {
	var scopes []ScopeInfo

	// Durchsuche alle Deklarationen
	ast.Inspect(file, func(n ast.Node) bool {
		// Suche nach Struct-Typen
		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}

		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			return true
		}

		// Prüfe ob es ein Scope ist (hat ecs: tags)
		scope := ScopeInfo{
			Name: typeSpec.Name.Name,
		}

		for _, field := range structType.Fields.List {
			if field.Tag == nil {
				continue
			}

			// Parse den Tag
			tag := reflect.StructTag(strings.Trim(field.Tag.Value, "`"))
			ecsTag := tag.Get("ecs")

			if ecsTag == "" {
				continue
			}

			// Extrahiere Typ-Name
			var typeName string
			if ident, ok := field.Type.(*ast.Ident); ok {
				typeName = ident.Name
			}

			// Parse die Tag-Optionen
			parts := strings.Split(ecsTag, ",")
			info := ComponentInfo{
				Name:     field.Names[0].Name,
				TypeName: typeName,
			}

			for _, part := range parts {
				part = strings.TrimSpace(part)
				switch part {
				case "read":
					info.IsRead = true
				case "write":
					info.IsWrite = true
				case "singleton":
					info.IsSingleton = true
				}
			}

			scope.Components = append(scope.Components, info)
		}

		if len(scope.Components) > 0 {
			scopes = append(scopes, scope)
		}

		return true
	})

	return scopes
}

func generateCode(scope ScopeInfo) {
	fmt.Printf("\n// Generated code for %s\n", scope.Name)
	fmt.Printf("func (s *%s) DependsOn() SystemDeps {\n", scope.Name)
	fmt.Println("    return SystemDeps{")

	// Generiere Reads
	fmt.Print("        Reads: []ComponentID{")
	var reads []string
	for _, comp := range scope.Components {
		if comp.IsRead && !comp.IsSingleton {
			reads = append(reads, fmt.Sprintf("ComponentID[%s]()", comp.TypeName))
		}
	}
	fmt.Printf("%s},\n", strings.Join(reads, ", "))

	// Generiere Writes
	fmt.Print("        Writes: []ComponentID{")
	var writes []string
	for _, comp := range scope.Components {
		if comp.IsWrite && !comp.IsSingleton {
			writes = append(writes, fmt.Sprintf("ComponentID[%s]()", comp.TypeName))
		}
	}
	fmt.Printf("%s},\n", strings.Join(writes, ", "))

	// Generiere Singletons
	fmt.Print("        Singletons: []SingletonID{")
	var singletons []string
	for _, comp := range scope.Components {
		if comp.IsSingleton {
			singletons = append(singletons, fmt.Sprintf("SingletonID[%s]()", comp.TypeName))
		}
	}
	fmt.Printf("%s},\n", strings.Join(singletons, ", "))

	fmt.Println("    }")
	fmt.Println("}")

	// Generiere Query-Methode
	fmt.Printf("\nfunc (s *%s) Query(fn func(e Entity", scope.Name)
	for _, comp := range scope.Components {
		if !comp.IsSingleton {
			fmt.Printf(", %s *%s", strings.ToLower(comp.Name[:1]), comp.TypeName)
		}
	}
	fmt.Println(")) {")
	fmt.Println("    // TODO: Implementiere Query-Logik")
	fmt.Println("}")
}
