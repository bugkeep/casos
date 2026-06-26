package object

import "fmt"

type HelmRepo struct {
	Id   int    `xorm:"pk autoincr" json:"id"`
	Name string `xorm:"varchar(100) unique notnull" json:"name"`
	Url  string `xorm:"varchar(500) notnull" json:"url"`
}

func GetHelmRepos() ([]*HelmRepo, error) {
	var repos []*HelmRepo
	err := ormer.Engine.Find(&repos)
	return repos, err
}

func AddHelmRepo(repo *HelmRepo) error {
	affected, err := ormer.Engine.Insert(repo)
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("repo not inserted")
	}
	return nil
}

func DeleteHelmRepo(id int) error {
	affected, err := ormer.Engine.ID(id).Delete(new(HelmRepo))
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("repo not found")
	}
	return nil
}
