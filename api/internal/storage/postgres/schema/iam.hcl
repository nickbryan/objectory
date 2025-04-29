schema "iam" {
  comment = "The identity and access management database."
}

table "identities" {
  schema = schema.iam
  column "id" {
    type = uuid
    null = false
  }
  column "email" {
    type = varchar(255)
    null = false
  }
  column "password" {
    type = text
    null = false
  }
  column "name" {
    type = varchar(255)
    null = false
  }
  column "created_at" {
    type = timestamp
    null = false
  }
  column "updated_at" {
    type = timestamp
    null = false
  }
  primary_key {
    columns = [
      column.id
    ]
  }
  index "email_unique" {
    columns = [
      column.email
    ]
    unique = true
  }
}
